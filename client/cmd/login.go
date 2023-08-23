package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"

	"github.com/netbirdio/netbird/client/internal"
	"github.com/netbirdio/netbird/client/internal/auth"
	"github.com/netbirdio/netbird/client/proto"
	"github.com/netbirdio/netbird/client/system"
	"github.com/netbirdio/netbird/util"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "login to the Netbird Management Service (first run)",
	RunE: func(cmd *cobra.Command, args []string) error {
		SetFlagsFromEnvVars(rootCmd)

		cmd.SetOut(cmd.OutOrStdout())

		err := util.InitLog(logLevel, "console")
		if err != nil {
			return fmt.Errorf("failed initializing log %v", err)
		}

		ctx := internal.CtxInitState(context.Background())

		if hostName != "" {
			// nolint
			ctx = context.WithValue(ctx, system.DeviceNameCtxKey, hostName)
		}

		// workaround to run without service
		if logFile == "console" {
			err = handleRebrand(cmd)
			if err != nil {
				return err
			}

			ic := internal.ConfigInput{
				ManagementURL: managementURL,
				AdminURL:      adminURL,
				ConfigPath:    configPath,
			}
			if preSharedKey != "" {
				ic.PreSharedKey = &preSharedKey
			}

			config, err := internal.UpdateOrCreateConfig(ic)
			if err != nil {
				return fmt.Errorf("get config file: %v", err)
			}

			config, _ = internal.UpdateOldManagementPort(ctx, config, configPath)

			err = foregroundLogin(ctx, cmd, config, setupKey)
			if err != nil {
				return fmt.Errorf("foreground login failed: %v", err)
			}
			cmd.Println("Logging successfully")
			return nil
		}

		conn, err := DialClientGRPCServer(ctx, daemonAddr)
		if err != nil {
			return fmt.Errorf("failed to connect to daemon error: %v\n"+
				"If the daemon is not running please run: "+
				"\nnetbird service install \nnetbird service start\n", err)
		}
		defer conn.Close()

		client := proto.NewDaemonServiceClient(conn)

		loginRequest := proto.LoginRequest{
			SetupKey:      setupKey,
			PreSharedKey:  preSharedKey,
			ManagementUrl: managementURL,
		}

		var loginErr error

		var loginResp *proto.LoginResponse

		err = WithBackOff(func() error {
			var backOffErr error
			loginResp, backOffErr = client.Login(ctx, &loginRequest)
			if s, ok := gstatus.FromError(backOffErr); ok && (s.Code() == codes.InvalidArgument ||
				s.Code() == codes.PermissionDenied ||
				s.Code() == codes.NotFound ||
				s.Code() == codes.Unimplemented) {
				loginErr = backOffErr
				return nil
			}
			return backOffErr
		})
		if err != nil {
			return fmt.Errorf("login backoff cycle failed: %v", err)
		}

		if loginErr != nil {
			return fmt.Errorf("login failed: %v", loginErr)
		}

		if loginResp.NeedsSSOLogin {
			openURL(cmd, loginResp.VerificationURIComplete, loginResp.UserCode)

			_, err = client.WaitSSOLogin(ctx, &proto.WaitSSOLoginRequest{UserCode: loginResp.UserCode})
			if err != nil {
				return fmt.Errorf("waiting sso login failed with: %v", err)
			}
		}

		cmd.Println("Logging successfully")

		return nil
	},
}

func foregroundLogin(ctx context.Context, cmd *cobra.Command, config *internal.Config, setupKey string) error {
	needsLogin := false

	err := WithBackOff(func() error {
		err := internal.Login(ctx, config, "", "")
		if s, ok := gstatus.FromError(err); ok && (s.Code() == codes.InvalidArgument || s.Code() == codes.PermissionDenied) {
			needsLogin = true
			return nil
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("backoff cycle failed: %v", err)
	}

	jwtToken := ""
	if setupKey == "" && needsLogin {
		tokenInfo, err := foregroundGetTokenInfo(ctx, cmd, config)
		if err != nil {
			return fmt.Errorf("interactive sso login failed: %v", err)
		}
		jwtToken = tokenInfo.GetTokenToUse()
	}

	err = WithBackOff(func() error {
		err := internal.Login(ctx, config, setupKey, jwtToken)
		if s, ok := gstatus.FromError(err); ok && (s.Code() == codes.InvalidArgument || s.Code() == codes.PermissionDenied) {
			return nil
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("backoff cycle failed: %v", err)
	}

	return nil
}

func foregroundGetTokenInfo(ctx context.Context, cmd *cobra.Command, config *internal.Config) (*auth.TokenInfo, error) {
	oAuthFlow, err := auth.NewOAuthFlow(ctx, config)
	if err != nil {
		return nil, err
	}

	flowInfo, err := oAuthFlow.RequestAuthInfo(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("getting a request OAuth flow info failed: %v", err)
	}

	openURL(cmd, flowInfo.VerificationURIComplete, flowInfo.UserCode)

	waitTimeout := time.Duration(flowInfo.ExpiresIn)
	waitCTX, c := context.WithTimeout(context.TODO(), waitTimeout*time.Second)
	defer c()

	tokenInfo, err := oAuthFlow.WaitToken(waitCTX, flowInfo)
	if err != nil {
		return nil, fmt.Errorf("waiting for browser login failed: %v", err)
	}

	return &tokenInfo, nil
}

func openURL(cmd *cobra.Command, verificationURIComplete, userCode string) {
	var codeMsg string
	if userCode != "" && !strings.Contains(verificationURIComplete, userCode) {
		codeMsg = fmt.Sprintf("and enter the code %s to authenticate.", userCode)
	}

	browserAuthMsg := "Please do the SSO login in your browser. \n" +
		"If your browser didn't open automatically, use this URL to log in:\n\n" +
		verificationURIComplete + " " + codeMsg

	setupKeyAuthMsg := "\nAlternatively, you may want to use a setup key, see:\n\n" +
		"https://docs.netbird.io/how-to/register-machines-using-setup-keys"

	authenticateUsingBrowser := func() {
		cmd.Println(browserAuthMsg)
		if err := open.Run(verificationURIComplete); err != nil {
			cmd.Println(setupKeyAuthMsg)
		}
	}

	switch runtime.GOOS {
	case "windows", "darwin":
		authenticateUsingBrowser()
	case "linux":
		if isLinuxRunningDesktop() {
			authenticateUsingBrowser()
		} else {
			// If current flow is PKCE, it implies the server is anticipating the redirect to localhost.
			// Devices lacking browser support are incompatible with this flow.Therefore,
			// these devices will need to resort to setup keys instead.
			if isPKCEFlow(verificationURIComplete) {
				cmd.Println("Please proceed with setting up this device using setup keys, see:\n\n" +
					"https://docs.netbird.io/how-to/register-machines-using-setup-keys")
			} else {
				cmd.Println(browserAuthMsg)
			}
		}
	}
}

// isLinuxRunningDesktop checks if a Linux OS is running desktop environment.
func isLinuxRunningDesktop() bool {
	for _, env := range os.Environ() {
		values := strings.Split(env, "=")
		if len(values) == 2 {
			key, value := values[0], values[1]
			if key == "XDG_CURRENT_DESKTOP" && value != "" {
				return true
			}
		}
	}
	return false
}

// isPKCEFlow determines if the PKCE flow is active or not,
// by checking the existence of redirect_uri inside the verification URL.
func isPKCEFlow(verificationURL string) bool {
	if verificationURL == "" {
		return false
	}
	return strings.Contains(verificationURL, "redirect_uri")
}

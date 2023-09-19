package idp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt"
	log "github.com/sirupsen/logrus"
	"goauthentik.io/api/v3"

	"github.com/netbirdio/netbird/management/server/telemetry"
)

// AuthentikManager authentik manager client instance.
type AuthentikManager struct {
	apiClient   *api.APIClient
	httpClient  ManagerHTTPClient
	credentials ManagerCredentials
	helper      ManagerHelper
	appMetrics  telemetry.AppMetrics
}

// AuthentikClientConfig authentik manager client configurations.
type AuthentikClientConfig struct {
	Issuer        string
	ClientID      string
	Username      string
	Password      string
	TokenEndpoint string
	GrantType     string
}

// AuthentikCredentials authentik authentication information.
type AuthentikCredentials struct {
	clientConfig AuthentikClientConfig
	helper       ManagerHelper
	httpClient   ManagerHTTPClient
	jwtToken     JWTToken
	mux          sync.Mutex
	appMetrics   telemetry.AppMetrics
}

// NewAuthentikManager creates a new instance of the AuthentikManager.
func NewAuthentikManager(config AuthentikClientConfig,
	appMetrics telemetry.AppMetrics) (*AuthentikManager, error) {
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.MaxIdleConns = 5

	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: httpTransport,
	}

	helper := JsonParser{}

	if config.ClientID == "" {
		return nil, fmt.Errorf("authentik IdP configuration is incomplete, clientID is missing")
	}

	if config.Username == "" {
		return nil, fmt.Errorf("authentik IdP configuration is incomplete, Username is missing")
	}

	if config.Password == "" {
		return nil, fmt.Errorf("authentik IdP configuration is incomplete, Password is missing")
	}

	if config.TokenEndpoint == "" {
		return nil, fmt.Errorf("authentik IdP configuration is incomplete, TokenEndpoint is missing")
	}

	if config.GrantType == "" {
		return nil, fmt.Errorf("authentik IdP configuration is incomplete, GrantType is missing")
	}

	// authentik client configuration
	issuerURL, err := url.Parse(config.Issuer)
	if err != nil {
		return nil, err
	}
	authentikConfig := api.NewConfiguration()
	authentikConfig.HTTPClient = httpClient
	authentikConfig.Host = issuerURL.Host
	authentikConfig.Scheme = issuerURL.Scheme

	credentials := &AuthentikCredentials{
		clientConfig: config,
		httpClient:   httpClient,
		helper:       helper,
		appMetrics:   appMetrics,
	}

	return &AuthentikManager{
		apiClient:   api.NewAPIClient(authentikConfig),
		httpClient:  httpClient,
		credentials: credentials,
		helper:      helper,
		appMetrics:  appMetrics,
	}, nil
}

// jwtStillValid returns true if the token still valid and have enough time to be used and get a response from authentik.
func (ac *AuthentikCredentials) jwtStillValid() bool {
	return !ac.jwtToken.expiresInTime.IsZero() && time.Now().Add(5*time.Second).Before(ac.jwtToken.expiresInTime)
}

// requestJWTToken performs request to get jwt token.
func (ac *AuthentikCredentials) requestJWTToken() (*http.Response, error) {
	data := url.Values{}
	data.Set("client_id", ac.clientConfig.ClientID)
	data.Set("username", ac.clientConfig.Username)
	data.Set("password", ac.clientConfig.Password)
	data.Set("grant_type", ac.clientConfig.GrantType)
	data.Set("scope", "goauthentik.io/api")

	payload := strings.NewReader(data.Encode())
	req, err := http.NewRequest(http.MethodPost, ac.clientConfig.TokenEndpoint, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Add("content-type", "application/x-www-form-urlencoded")

	log.Debug("requesting new jwt token for authentik idp manager")

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		if ac.appMetrics != nil {
			ac.appMetrics.IDPMetrics().CountRequestError()
		}

		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to get authentik token, statusCode %d", resp.StatusCode)
	}

	return resp, nil
}

// parseRequestJWTResponse parses jwt raw response body and extracts token and expires in seconds
func (ac *AuthentikCredentials) parseRequestJWTResponse(rawBody io.ReadCloser) (JWTToken, error) {
	jwtToken := JWTToken{}
	body, err := io.ReadAll(rawBody)
	if err != nil {
		return jwtToken, err
	}

	err = ac.helper.Unmarshal(body, &jwtToken)
	if err != nil {
		return jwtToken, err
	}

	if jwtToken.ExpiresIn == 0 && jwtToken.AccessToken == "" {
		return jwtToken, fmt.Errorf("error while reading response body, expires_in: %d and access_token: %s", jwtToken.ExpiresIn, jwtToken.AccessToken)
	}

	data, err := jwt.DecodeSegment(strings.Split(jwtToken.AccessToken, ".")[1])
	if err != nil {
		return jwtToken, err
	}

	// Exp maps into exp from jwt token
	var IssuedAt struct{ Exp int64 }
	err = ac.helper.Unmarshal(data, &IssuedAt)
	if err != nil {
		return jwtToken, err
	}
	jwtToken.expiresInTime = time.Unix(IssuedAt.Exp, 0)

	return jwtToken, nil
}

// Authenticate retrieves access token to use the authentik management API.
func (ac *AuthentikCredentials) Authenticate() (JWTToken, error) {
	ac.mux.Lock()
	defer ac.mux.Unlock()

	if ac.appMetrics != nil {
		ac.appMetrics.IDPMetrics().CountAuthenticate()
	}

	// reuse the token without requesting a new one if it is not expired,
	// and if expiry time is sufficient time available to make a request.
	if ac.jwtStillValid() {
		return ac.jwtToken, nil
	}

	resp, err := ac.requestJWTToken()
	if err != nil {
		return ac.jwtToken, err
	}
	defer resp.Body.Close()

	jwtToken, err := ac.parseRequestJWTResponse(resp.Body)
	if err != nil {
		return ac.jwtToken, err
	}

	ac.jwtToken = jwtToken

	return ac.jwtToken, nil
}

// UpdateUserAppMetadata updates user app metadata based on userID and metadata map.
func (am *AuthentikManager) UpdateUserAppMetadata(userID string, appMetadata AppMetadata) error {
	ctx, err := am.authenticationContext()
	if err != nil {
		return err
	}

	userPk, err := strconv.ParseInt(userID, 10, 32)
	if err != nil {
		return err
	}

	var pendingInvite bool
	if appMetadata.WTPendingInvite != nil {
		pendingInvite = *appMetadata.WTPendingInvite
	}

	patchedUserReq := api.PatchedUserRequest{
		Attributes: map[string]interface{}{
			wtAccountID:     appMetadata.WTAccountID,
			wtPendingInvite: pendingInvite,
		},
	}
	_, resp, err := am.apiClient.CoreApi.CoreUsersPartialUpdate(ctx, int32(userPk)).
		PatchedUserRequest(patchedUserReq).
		Execute()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if am.appMetrics != nil {
		am.appMetrics.IDPMetrics().CountUpdateUserAppMetadata()
	}

	if resp.StatusCode != http.StatusOK {
		if am.appMetrics != nil {
			am.appMetrics.IDPMetrics().CountRequestStatusError()
		}
		return fmt.Errorf("unable to update user %s, statusCode %d", userID, resp.StatusCode)
	}

	return nil
}

// GetUserDataByID requests user data from authentik via ID.
func (am *AuthentikManager) GetUserDataByID(userID string, appMetadata AppMetadata) (*UserData, error) {
	ctx, err := am.authenticationContext()
	if err != nil {
		return nil, err
	}

	userPk, err := strconv.ParseInt(userID, 10, 32)
	if err != nil {
		return nil, err
	}

	user, resp, err := am.apiClient.CoreApi.CoreUsersRetrieve(ctx, int32(userPk)).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if am.appMetrics != nil {
		am.appMetrics.IDPMetrics().CountGetUserDataByID()
	}

	if resp.StatusCode != http.StatusOK {
		if am.appMetrics != nil {
			am.appMetrics.IDPMetrics().CountRequestStatusError()
		}
		return nil, fmt.Errorf("unable to get user %s, statusCode %d", userID, resp.StatusCode)
	}

	return parseAuthentikUser(*user)
}

// GetAccount returns all the users for a given profile.
func (am *AuthentikManager) GetAccount(accountID string) ([]*UserData, error) {
	ctx, err := am.authenticationContext()
	if err != nil {
		return nil, err
	}

	accountFilter := fmt.Sprintf("{%q:%q}", wtAccountID, accountID)
	userList, resp, err := am.apiClient.CoreApi.CoreUsersList(ctx).Attributes(accountFilter).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if am.appMetrics != nil {
		am.appMetrics.IDPMetrics().CountGetAccount()
	}

	if resp.StatusCode != http.StatusOK {
		if am.appMetrics != nil {
			am.appMetrics.IDPMetrics().CountRequestStatusError()
		}
		return nil, fmt.Errorf("unable to get account %s users, statusCode %d", accountID, resp.StatusCode)
	}

	users := make([]*UserData, 0)
	for _, user := range userList.Results {
		userData, err := parseAuthentikUser(user)
		if err != nil {
			return nil, err
		}
		users = append(users, userData)
	}

	return users, nil
}

// GetAllAccounts gets all registered accounts with corresponding user data.
// It returns a list of users indexed by accountID.
func (am *AuthentikManager) GetAllAccounts() (map[string][]*UserData, error) {
	ctx, err := am.authenticationContext()
	if err != nil {
		return nil, err
	}

	userList, resp, err := am.apiClient.CoreApi.CoreUsersList(ctx).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if am.appMetrics != nil {
		am.appMetrics.IDPMetrics().CountGetAllAccounts()
	}

	if resp.StatusCode != http.StatusOK {
		if am.appMetrics != nil {
			am.appMetrics.IDPMetrics().CountRequestStatusError()
		}
		return nil, fmt.Errorf("unable to get all accounts, statusCode %d", resp.StatusCode)
	}

	indexedUsers := make(map[string][]*UserData)
	for _, user := range userList.Results {
		userData, err := parseAuthentikUser(user)
		if err != nil {
			return nil, err
		}

		accountID := userData.AppMetadata.WTAccountID
		if accountID != "" {
			if _, ok := indexedUsers[accountID]; !ok {
				indexedUsers[accountID] = make([]*UserData, 0)
			}
			indexedUsers[accountID] = append(indexedUsers[accountID], userData)
		}
	}

	return indexedUsers, nil
}

// CreateUser creates a new user in authentik Idp and sends an invitation.
func (am *AuthentikManager) CreateUser(email, name, accountID, invitedByEmail string) (*UserData, error) {
	ctx, err := am.authenticationContext()
	if err != nil {
		return nil, err
	}

	groupID, err := am.getUserGroupByName("netbird")
	if err != nil {
		return nil, err
	}

	defaultBoolValue := true
	createUserRequest := api.UserRequest{
		Email:    &email,
		Name:     name,
		IsActive: &defaultBoolValue,
		Groups:   []string{groupID},
		Username: email,
		Attributes: map[string]interface{}{
			wtAccountID:     accountID,
			wtPendingInvite: &defaultBoolValue,
		},
	}
	user, resp, err := am.apiClient.CoreApi.CoreUsersCreate(ctx).UserRequest(createUserRequest).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if am.appMetrics != nil {
		am.appMetrics.IDPMetrics().CountCreateUser()
	}

	if resp.StatusCode != http.StatusCreated {
		if am.appMetrics != nil {
			am.appMetrics.IDPMetrics().CountRequestStatusError()
		}
		return nil, fmt.Errorf("unable to create user, statusCode %d", resp.StatusCode)
	}

	return parseAuthentikUser(*user)
}

// GetUserByEmail searches users with a given email.
// If no users have been found, this function returns an empty list.
func (am *AuthentikManager) GetUserByEmail(email string) ([]*UserData, error) {
	ctx, err := am.authenticationContext()
	if err != nil {
		return nil, err
	}

	userList, resp, err := am.apiClient.CoreApi.CoreUsersList(ctx).Email(email).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if am.appMetrics != nil {
		am.appMetrics.IDPMetrics().CountGetUserByEmail()
	}

	if resp.StatusCode != http.StatusOK {
		if am.appMetrics != nil {
			am.appMetrics.IDPMetrics().CountRequestStatusError()
		}
		return nil, fmt.Errorf("unable to get user %s, statusCode %d", email, resp.StatusCode)
	}

	users := make([]*UserData, 0)
	for _, user := range userList.Results {
		userData, err := parseAuthentikUser(user)
		if err != nil {
			return nil, err
		}
		users = append(users, userData)
	}

	return users, nil
}

// InviteUserByID resend invitations to users who haven't activated,
// their accounts prior to the expiration period.
func (am *AuthentikManager) InviteUserByID(_ string) error {
	return fmt.Errorf("method InviteUserByID not implemented")
}

// DeleteUser from Authentik
func (am *AuthentikManager) DeleteUser(userID string) error {
	ctx, err := am.authenticationContext()
	if err != nil {
		return err
	}

	userPk, err := strconv.ParseInt(userID, 10, 32)
	if err != nil {
		return err
	}

	resp, err := am.apiClient.CoreApi.CoreUsersDestroy(ctx, int32(userPk)).Execute()
	if err != nil {
		return err
	}
	defer resp.Body.Close() // nolint

	if am.appMetrics != nil {
		am.appMetrics.IDPMetrics().CountDeleteUser()
	}

	if resp.StatusCode != http.StatusNoContent {
		if am.appMetrics != nil {
			am.appMetrics.IDPMetrics().CountRequestStatusError()
		}
		return fmt.Errorf("unable to delete user %s, statusCode %d", userID, resp.StatusCode)
	}

	return nil
}

func (am *AuthentikManager) authenticationContext() (context.Context, error) {
	jwtToken, err := am.credentials.Authenticate()
	if err != nil {
		return nil, err
	}

	value := map[string]api.APIKey{
		"authentik": {
			Key:    jwtToken.AccessToken,
			Prefix: jwtToken.TokenType,
		},
	}
	return context.WithValue(context.Background(), api.ContextAPIKeys, value), nil
}

// getUserGroupByName retrieves the user group for assigning new users.
// If the group is not found, a new group with the specified name will be created.
func (am *AuthentikManager) getUserGroupByName(name string) (string, error) {
	ctx, err := am.authenticationContext()
	if err != nil {
		return "", err
	}

	groupList, resp, err := am.apiClient.CoreApi.CoreGroupsList(ctx).Name(name).Execute()
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if groupList != nil {
		if len(groupList.Results) > 0 {
			return groupList.Results[0].Pk, nil
		}
	}

	createGroupRequest := api.GroupRequest{Name: name}
	group, resp, err := am.apiClient.CoreApi.CoreGroupsCreate(ctx).GroupRequest(createGroupRequest).Execute()
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unable to create user group, statusCode: %d", resp.StatusCode)
	}

	return group.Pk, nil
}

func parseAuthentikUser(user api.User) (*UserData, error) {
	var attributes struct {
		AccountID     string `json:"wt_account_id"`
		PendingInvite bool   `json:"wt_pending_invite"`
	}

	helper := JsonParser{}
	buf, err := helper.Marshal(user.Attributes)
	if err != nil {
		return nil, err
	}

	err = helper.Unmarshal(buf, &attributes)
	if err != nil {
		return nil, err
	}

	return &UserData{
		Email: *user.Email,
		Name:  user.Name,
		ID:    strconv.FormatInt(int64(user.Pk), 10),
		AppMetadata: AppMetadata{
			WTAccountID:     attributes.AccountID,
			WTPendingInvite: &attributes.PendingInvite,
		},
	}, nil
}

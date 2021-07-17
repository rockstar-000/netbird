package cmd

import (
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	mgmt "github.com/wiretrustee/wiretrustee/management"
	mgmtProto "github.com/wiretrustee/wiretrustee/management/proto"
	"google.golang.org/grpc"
	"net"
)

var (
	mgmtPort    int
	mgmtDataDir string

	mgmtCmd = &cobra.Command{
		Use:   "management",
		Short: "start Wiretrustee Management Server",
		Run: func(cmd *cobra.Command, args []string) {
			flag.Parse()

			lis, err := net.Listen("tcp", fmt.Sprintf(":%d", mgmtPort))
			if err != nil {
				log.Fatalf("failed to listen: %v", err)
			}

			if err != nil {
				log.Fatalf("failed to listen: %v", err)
			}

			var opts []grpc.ServerOption
			grpcServer := grpc.NewServer(opts...)
			defer grpcServer.Stop()

			server, err := mgmt.NewServer(mgmtDataDir)
			if err != nil {
				log.Fatalf("failed creating new server: %v", err)
				panic(err)
			}
			mgmtProto.RegisterManagementServiceServer(grpcServer, server)
			log.Printf("started server: localhost:%v", mgmtPort)
			go func() {
				if err = grpcServer.Serve(lis); err != nil {
					log.Fatalf("failed to serve: %v", err)
				}
			}()

			SetupCloseHandler()
			<-stopCh
			log.Println("Receive signal to stop running Management server")
		},
	}
)

func init() {
	mgmtCmd.PersistentFlags().IntVar(&mgmtPort, "port", 33073, "Server port to listen on (e.g. 33073)")
	mgmtCmd.PersistentFlags().StringVar(&mgmtDataDir, "datadir", "/data", "Server data directory location (e.g. /data")

}

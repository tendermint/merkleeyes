// Copyright 2017 Tendermint. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/abci/server"

	cmn "github.com/tendermint/tmlibs/common"

	application "github.com/tendermint/merkleeyes/app"
)

var (
	address string
	abci    string
	cache   int
)

var startCmd = &cobra.Command{
	Run:   StartServer,
	Use:   "start",
	Short: "Start the MerkleEyes server",
	Long:  `Startup the MerkleEyes ABCi app`,
}

func init() {
	RootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVarP(&address, "address", "l", "tcp://127.0.0.1:46658", "MerkleEyes server listen address")
	startCmd.Flags().StringVarP(&abci, "abci", "a", "socket", "socket | grpc")
	startCmd.Flags().IntVarP(&cache, "cache", "c", 0, "database cache size")
}

func StartServer(cmd *cobra.Command, args []string) {
	dbName := viper.GetString(FlagDBName)
	app := application.NewMerkleEyesApp(dbName, cache)
	server, err := server.NewServer(address, abci, app)

	if err != nil {
		cmn.Exit(err.Error())
	}
	_, err = server.Start()
	if err != nil {
		cmn.Exit(err.Error())
	}

	cmn.TrapSignal(func() {
		app.CloseDB()
		server.Stop()
	})
}

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
	"github.com/tendermint/merkleeyes/app"
	"github.com/tendermint/tmlibs/cli"
)

const (
	defaultLogLevel = "error"
	FlagLogLevel    = "log_level"
	// TODO: fix up these flag names when we do a minor release
	FlagDBName = "dbName"
	FlagDBType = "dbType"
)

var RootCmd = &cobra.Command{
	Use:   "merkleeyes",
	Short: "Merkleeyes server",
	Long: `Merkleeyes server and other tools

Including:
        - Start the Merkleeyes server
	- Benchmark to check the underlying performance of the databases.
	- Dump to list the full contents of any persistent go-merkle database.
	`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		level := viper.GetString(FlagLogLevel)
		err = app.SetLogLevel(level)
		if err != nil {
			return err
		}
		if viper.GetBool(cli.TraceFlag) {
			app.SetTraceLogger()
		}
		return nil
	},
}

func init() {
	RootCmd.PersistentFlags().StringP(FlagDBType, "t", "goleveldb", "type of backing db")
	RootCmd.PersistentFlags().StringP(FlagDBName, "d", "", "database name")
	RootCmd.PersistentFlags().String(FlagLogLevel, defaultLogLevel, "Log level")
}

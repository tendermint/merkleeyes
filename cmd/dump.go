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
	"fmt"
	"path"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cmn "github.com/tendermint/tmlibs/common"
	db "github.com/tendermint/tmlibs/db"

	"github.com/tendermint/merkleeyes/iavl"
)

var (
	dbDir     string
	verbose   bool
	cacheSize int
)

var dumpCmd = &cobra.Command{
	Run:   DumpDatabase,
	Use:   "dump",
	Short: "Dump a database",
	Long:  `Dump all of the data for an underlying persistent database`,
}

func init() {
	RootCmd.AddCommand(dumpCmd)
	dumpCmd.Flags().StringVarP(&dbDir, "path", "p", "./", "Dir path to DB")
	dumpCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print everything")
	dumpCmd.Flags().IntVarP(&cacheSize, "cache", "c", 10000, "Size of the Cache")
}

func DumpDatabase(cmd *cobra.Command, args []string) {
	dbName := viper.GetString(FlagDBName)
	dbType := viper.GetString(FlagDBType)

	if dbName == "" {
		dbName = "merkleeyes"
	}

	dbPath := path.Join(dbDir, dbName+".db")

	if !cmn.FileExists(dbPath) {
		cmn.Exit("No existing database: " + dbPath)
	}

	if verbose {
		fmt.Printf("Dumping DB %s (%s)...\n", dbName, dbType)
	}

	database := db.NewDB(dbName, db.LevelDBBackendStr, "./")

	if verbose {
		fmt.Printf("Database: %v\n", database)
	}

	tree := iavl.NewIAVLTree(cacheSize, database)

	if verbose {
		fmt.Printf("Tree: %v\n", tree)
	}

	tree.Dump(verbose, nil)
}

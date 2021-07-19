// Copyright 2017 Pilosa Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/molecula/featurebase/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func NewRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	rc := &cobra.Command{
		Use: "pilosa",
		// TODO: These short/long descriptions could use some updating.
		Short: "Pilosa - A Distributed In-memory Binary Bitmap Index.",
		Long: `Pilosa is a fast index to turbocharge your database.

This binary contains Pilosa itself, as well as common
tools for administering Pilosa, importing/exporting data,
backing up, and more. Complete documentation is available
at https://docs.molecula.cloud/.

` + pilosa.VersionInfo(false) + "\n",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			v := viper.New()
			err := setAllConfig(v, cmd.Flags())
			if err != nil {
				return err
			}

			// return "dry run" error if "dry-run" flag is set
			ret, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return fmt.Errorf("problem getting dry-run flag: %v", err)
			}
			if ret {
				if cmd.Parent() != nil {
					return fmt.Errorf("dry run")
				}
			}

			return nil
		},
	}
	rc.PersistentFlags().Bool("dry-run", false, "stop before executing")
	_ = rc.PersistentFlags().MarkHidden("dry-run")
	rc.PersistentFlags().StringP("config", "c", "", "Configuration file to read from.")

	rc.AddCommand(newChkSumCommand(stdin, stdout, stderr))
	rc.AddCommand(newBackupCommand(stdin, stdout, stderr))
	rc.AddCommand(newRestoreCommand(stdin, stdout, stderr))
	rc.AddCommand(newCheckCommand(stdin, stdout, stderr))
	rc.AddCommand(newConfigCommand(stdin, stdout, stderr))
	rc.AddCommand(newExportCommand(stdin, stdout, stderr))
	rc.AddCommand(newGenerateConfigCommand(stdin, stdout, stderr))
	rc.AddCommand(newImportCommand(stdin, stdout, stderr))
	rc.AddCommand(newInspectCommand(stdin, stdout, stderr))
	rc.AddCommand(newRBFCommand(stdin, stdout, stderr))
	rc.AddCommand(newServeCmd(stdin, stdout, stderr))
	rc.AddCommand(newHolderCmd(stdin, stdout, stderr))

	rc.SetOutput(stderr)
	return rc
}

// setAllConfig takes a FlagSet to be the definition of all configuration
// options, as well as their defaults. It then reads from the command line, the
// environment, and a config file (if specified), and applies the configuration
// in that priority order. Since each flag in the set contains a pointer to
// where its value should be stored, setAllConfig can directly modify the value
// of each config variable.
//
// setAllConfig looks for environment variables which are capitalized versions
// of the flag names with dashes replaced by underscores, and prefixed with
// envPrefix plus an underscore.
func setAllConfig(v *viper.Viper, flags *pflag.FlagSet) error { // nolint: unparam
	// add cmd line flag def to viper
	err := v.BindPFlags(flags)
	if err != nil {
		return err
	}

	envPrefix := "PILOSA"
	rename := v.GetBool("future.rename")
	if rename {
		envPrefix = "FEATUREBASE"
	}

	// add env to viper
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	c := v.GetString("config")
	var flagErr error
	validTags := make(map[string]bool)
	flags.VisitAll(func(f *pflag.Flag) {
		validTags[f.Name] = true
	})

	// add config file to viper
	if c != "" {
		v.SetConfigFile(c)
		v.SetConfigType("toml")
		err := v.ReadInConfig()
		if err != nil {
			return fmt.Errorf("error reading configuration file '%s': %v", c, err)
		}

		for _, key := range v.AllKeys() {
			if _, ok := validTags[key]; !ok {
				return fmt.Errorf("invalid option in configuration file: %v", key)
			}
		}

	}

	// set all values from viper
	flags.VisitAll(func(f *pflag.Flag) {
		if flagErr != nil {
			return
		}
		var value string
		if f.Value.Type() == "stringSlice" {
			// special handling is needed for stringSlice as v.GetString will
			// always return "" in the case that the value is an actual string
			// slice from a config file rather than a comma separated string
			// from a flag or env var.
			vss := v.GetStringSlice(f.Name)
			value = strings.Join(vss, ",")
		} else {
			value = v.GetString(f.Name)
		}

		if f.Changed {
			// If f.Changed is true, that means the value has already been set
			// by a flag, and we don't need to ask viper for it since the flag
			// is the highest priority. This works around a problem with string
			// slices where f.Value.Set(csvString) would cause the elements of
			// csvString to be appended to the existing value rather than
			// replacing it.
			return
		}
		flagErr = f.Value.Set(value)
	})
	return flagErr
}

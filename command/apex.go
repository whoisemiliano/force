package command

import (
	"fmt"
	"io/ioutil"
	"os"

	. "github.com/ForceCLI/force/error"
	"github.com/spf13/cobra"
)

func init() {
	apexCmd.Flags().BoolP("test", "t", false, "run in test context")
	RootCmd.AddCommand(apexCmd)
}

var apexCmd = &cobra.Command{
	Use:   "apex [file]",
	Short: "Execute anonymous Apex code",
	Example: `
  force apex ~/test.apex

  force apex
  >> Start typing Apex code; press CTRL-D(for Mac/Linux) / Ctrl-Z (for Windows) when finished
  `,
	Run: func(cmd *cobra.Command, args []string) {
		testContext, _ := cmd.Flags().GetBool("test")
		switch len(args) {
		case 1:
			runApexInFile(args[0], testContext)
		case 0:
			runApexFromStdin(testContext)
		default:
			fmt.Println("Got test indication.  DEPRECATED.")
			getTestCoverage(args[1])
		}
	},
}

func runApexFromStdin(testContext bool) {
	fmt.Println(">> Start typing Apex code; press CTRL-D(for Mac/Linux) / Ctrl-Z (for Windows) when finished")
	code, err := ioutil.ReadAll(os.Stdin)
	fmt.Println("\n\n>> Executing code...")
	var output string
	if testContext {
		output, err = executeAsTest(code)
	} else {
		output, err = execute(code)
	}
	if err != nil {
		ErrorAndExit(err.Error())
	}
	fmt.Println(output)
}

func runApexInFile(filename string, testContext bool) {
	code, err := ioutil.ReadFile(filename)
	if err != nil {
		ErrorAndExit(err.Error())
	}
	var output string
	if testContext {
		output, err = executeAsTest(code)
	} else {
		output, err = execute(code)
	}
	if err != nil {
		ErrorAndExit(err.Error())
	}
	fmt.Println(output)
}

func executeAsTest(code []byte) (string, error) {
	return force.Partner.ExecuteAnonymousTest(string(code))
}

func execute(code []byte) (string, error) {
	return force.Partner.ExecuteAnonymous(string(code))
}

func getTestCoverage(apexclass string) {
	fmt.Println(apexclass)
	err := force.GetCodeCoverage("", apexclass)
	if err != nil {
		ErrorAndExit(err.Error())
	}
}

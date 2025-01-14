package command

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ForceCLI/force/config"
	. "github.com/ForceCLI/force/error"
	. "github.com/ForceCLI/force/lib"
	"github.com/spf13/cobra"
)

var (
	namePaths     = make(map[string]string)
	resourcepaths metaName
	metaFolder    string
)

func init() {
	// Deploy options
	pushCmd.Flags().BoolP("rollbackonerror", "r", false, "roll back deployment on error")
	pushCmd.Flags().Bool("runalltests", false, "run all tests (equivalent to --testlevel RunAllTestsInOrg)")
	pushCmd.Flags().StringP("testlevel", "l", "NoTestRun", "test level")
	pushCmd.Flags().BoolP("checkonly", "c", false, "check only deploy")
	pushCmd.Flags().BoolP("purgeondelete", "p", false, "purge metadata from org on delete")
	pushCmd.Flags().BoolP("allowmissingfiles", "m", false, "set allow missing files")
	pushCmd.Flags().BoolP("autoupdatepackage", "u", false, "set auto update package")
	pushCmd.Flags().BoolP("ignorewarnings", "i", false, "ignore warnings")

	// Ways to push
	pushCmd.Flags().StringSliceVarP(&resourcepaths, "filepath", "f", []string{}, "Path to resource(s)")
	pushCmd.Flags().StringSlice("test", []string{}, "Test(s) to run")
	pushCmd.Flags().StringP("type", "t", "", "Metatdata type")
	pushCmd.Flags().StringSliceVarP(&metadataName, "name", "n", []string{}, "name of metadata object")
	RootCmd.AddCommand(pushCmd)
}

var pushCmd = &cobra.Command{
	Use:   "push [flags]",
	Short: "Deploy metadata from a local directory",
	Long: `
Deploy artifact from a local directory
<metadata>: Accepts either actual directory name or Metadata type
File path can be specified as - to read from stdin; see examples
`,

	Example: `
  force push -t StaticResource -n MyResource
  force push -t ApexClass
  force push -f metadata/classes/MyClass.cls
  force push -checkonly -test MyClass_Test metadata/classes/MyClass.cls
  force push -n MyApex -n MyObject__c
  git diff HEAD^ --name-only --diff-filter=ACM | force push -f -
`,
	DisableFlagsInUseLine: false,
	Run: func(cmd *cobra.Command, args []string) {
		options := getDeploymentOptions(cmd)
		metadataType, _ := cmd.Flags().GetString("type")
		runPush(metadataType, args, options)
	},
}

func replaceComponentWithBundle(inputPathToFile string) string {
	dirPart, filePart := filepath.Split(inputPathToFile)
	dirPart = filepath.Dir(dirPart)
	if strings.Contains(dirPart, "aura") && filepath.Ext(filePart) != "" && filepath.Base(filepath.Dir(dirPart)) == "aura" {
		inputPathToFile = dirPart
	}
	if strings.Contains(dirPart, "lwc") && filepath.Ext(filePart) != "" && filepath.Base(filepath.Dir(dirPart)) == "lwc" {
		inputPathToFile = dirPart
	}
	return inputPathToFile
}

func runPush(metadataType string, args []string, options ForceDeployOptions) {
	if strings.ToLower(metadataType) == "package" {
		pushPackage(options)
		return
	}
	// Treat trailing args as file paths
	resourcepaths = append(resourcepaths, args...)

	if len(resourcepaths) == 1 && resourcepaths[0] == "-" {
		resourcepaths = make(metaName, 0)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			resourcepaths = append(resourcepaths, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			ErrorAndExit("Error reading stdin")
		}
	}

	if len(metadataType) == 0 && len(resourcepaths) == 0 {
		ErrorAndExit("Nothing to push. Please specify metadata components to deploy.")
	}

	if len(resourcepaths) > 0 {
		// It's not a package but does have a path. This could be a path to a file
		// or to a folder. If it is a folder, we pickup the resources a different
		// way than if it's a file.

		// Replace aura/lwc file reference with full bundle folder because only the
		// main component can be deployed by itself.
		resourcepathsToPush := make(metaName, 0)
		for _, fsPath := range resourcepaths {
			resourcepathsToPush = append(resourcepathsToPush, replaceComponentWithBundle(fsPath))
		}
		resourcepaths = resourcepathsToPush

		validatePushByMetadataTypeCommand(metadataType)
		PushByPaths(force, resourcepaths, false, namePaths, &options)
	} else {
		if len(metadataName) > 0 {
			if len(metadataType) != 0 {
				validatePushByMetadataTypeCommand(metadataType)
				pushByMetadataType(options)
			} else {
				ErrorAndExit("The -type (-t) parameter is required.")
			}
		} else {
			validatePushByMetadataTypeCommand(metadataType)
			pushByMetadataType(options)
		}
	}
}

func isValidMetadataType(metadataType string) {
	fmt.Printf("Validating and deploying push...\n")
	// Look to see if we can find any resource for that metadata type
	root, err := config.GetSourceDir()
	ExitIfNoSourceDir(err)
	metaFolder = findMetadataTypeFolder(metadataType, root)
	if metaFolder == "" {
		ErrorAndExit("No folders that contain %s metadata could be found.", metadataType)
	}
}

func metadataExists() {
	if len(metadataName) == 0 {
		return
	} else {
		valid := true
		message := ""
		// Go throug the metadata folder to find the named resources
		for _, name := range metadataName {
			if len(wildCardSearch(metaFolder, strings.Split(name, ".")[0])) == 0 {
				message += fmt.Sprintf("\nINVALID: No resource named %s found in %s", name, metaFolder)
				valid = false
			}
		}
		if !valid {
			ErrorAndExit(message)
		}
	}
}

func validatePushByMetadataTypeCommand(metadataType string) {
	// TODO: Is this needed?
	isValidMetadataType(metadataType)
	metadataExists()
}

func wildCardSearch(metaFolder string, name string) []string {
	files, err := ioutil.ReadDir(metaFolder)
	if err != nil {
		ErrorAndExit(err.Error())
	}

	var ret []string
	for _, s := range files {
		ss := s.Name()
		ss = strings.Split(ss, ".")[0]
		if ss == name {
			ret = append(ret, ss)
		}
	}
	return ret
}

func pushPackage(options ForceDeployOptions) {
	if len(resourcepaths) == 0 {
		var packageFolder = findPackageFolder(metadataName[0])
		zipResource(packageFolder, metadataName[0])
		resourcepaths = append(resourcepaths, packageFolder+".resource")
		//var dir, _ = os.Getwd();
		//ErrorAndExit(fmt.Sprintf("No resource path sepcified. %s, %s", metadataName[0], dir))
	}
	DeployPackage(force, resourcepaths, &options)
}

// Return the name of the first element of an XML file. We need this
// because the metadata xml uses the metadata type as the first element
// in the metadata xml definition. Could be a better way of doing this.
func getMDTypeFromXml(path string) (mdtype string, err error) {
	xmlFile, err := ioutil.ReadFile(path)
	mdtype = getFirstXmlElement(xmlFile)
	return
}

// Helper function to read the first element of an XML file.
func getFirstXmlElement(xmlFile []byte) (firstElement string) {
	decoder := xml.NewDecoder(strings.NewReader(string(xmlFile)))
	for {
		token, _ := decoder.Token()
		if token == nil {
			break
		}
		switch startElement := token.(type) {
		case xml.StartElement:
			firstElement = startElement.Name.Local
			return
		}
	}
	return
}

// Look for xml files. When one is found, check the first element of the
// XML. It should be the metadata type as expected by the platform.  See
// if it matches the type passed in on mdtype, and if so, return the folder
// that contains the xml file, then bail out.  If no file is found for the
// passed in type, then folder is empty.
func findMetadataTypeFolder(mdtype string, root string) (folder string) {
	filepath.Walk(root, func(path string, f os.FileInfo, err error) error {
		firstEl, _ := getMDTypeFromXml(path)
		if firstEl == mdtype {
			// This is sufficient for MD that does not have sub folders (classes, pages, etc)
			// It is NOT sufficient for aura bundles
			if mdtype == "AuraDefinitionBundle" || mdtype == "LightningComponentBundle" {
				// Need the parent of this folder to get all aura bundles in the directory
				folder = filepath.Dir(filepath.Dir(path))
			} else {
				folder = filepath.Dir(path)
			}
			return errors.New("walk canceled")
		}

		return nil
	})
	return
}

func findPackageFolder(packageName string) (folder string) {
	var wd, _ = os.Getwd()
	// We need to start at the metadata folder, go down first
	folder = findMetadataFolder(wd)
	if len(folder) == 0 {
		// Didn't find it, error out
		fmt.Println("Could not find metadata folder.")
	}
	if _, err := os.Stat(filepath.Join(folder, packageName)); err == nil {
		folder = filepath.Join(folder, packageName)
	}
	return
}

func findMetadataFolder(dir string) (folderPath string) {
	filepath.Walk(dir, func(path string, f os.FileInfo, err error) error {
		if filepath.Base(path) == "metadata" {
			folderPath = path
			return errors.New("walk cancelled")
		}
		return nil
	})
	if len(folderPath) == 0 {
		// not down, so, go up
		for dir != string(os.PathSeparator) {
			dir = filepath.Dir(dir)
			if filepath.Base(dir) == "metadata" {
				folderPath = dir
				return
			}
		}
	}
	return
}

func FilenameMatchesMetadataName(filename string, metadataName string) bool {
	// Strip off the extension, plus "-meta.xml" if it's appended to the
	// extension
	re := regexp.MustCompile(`\.[^.]+(-meta\.xml)?$`)
	trimmed := re.ReplaceAllString(filename, "")
	return trimmed == metadataName
}

// This method will use the type that is passed to the -type flag to find all
// metadata that matches that type.  It will also filter on the metadata
// name(s) passed on the -name flag(s). This method also looks for unpacked
// static resource so that it can repack them and update the actual ".resource"
// file.
func pushByMetadataType(options ForceDeployOptions) {
	// TODO: get all files that match these types and make a list out of them

	// Walk the metaFolder obtained during validation and compile a list of resources
	// to be added to the package.
	var files []string

	// Handle aura/lwc bundles separately
	if filepath.Base(metaFolder) == "aura" || filepath.Base(metaFolder) == "lwc" {
		cur := ""
		filepath.Walk(metaFolder, func(path string, f os.FileInfo, err error) error {
			if f.IsDir() && cur != f.Name() {
				cur = f.Name()
				fmt.Printf("Pushing " + f.Name() + "\n")
			}
			if (f.Name() != "aura" && f.Name() != "lwc") && strings.ToLower(f.Name()) != ".ds_store" && f.IsDir() {
				absPath, _ := filepath.Abs(path)
				pushAuraComponentByPath(absPath)
			}
			return nil
		})
		return
	}

	filepath.Walk(metaFolder, func(path string, f os.FileInfo, err error) error {
		// Check to see if this is a folder. This will be the case with static resources
		// that have been unpacked.  Not entirely sure if this is the only time we will
		// find a folder inside a metadata type folder.
		if f.IsDir() {
			if f.Name() != "aura" && filepath.Base(filepath.Dir(path)) != "aura" && filepath.Base(filepath.Dir(filepath.Dir(path))) != "aura" &&
				f.Name() != "lwc" && filepath.Base(filepath.Dir(path)) != "lwc" && filepath.Base(filepath.Dir(filepath.Dir(path))) != "lwc" {
				// Check to see if any names where specified in the -name flag
				if len(metadataName) == 0 {
					// Take all
					zipResource(path, "")
				} else {
					for _, name := range metadataName {
						fname := filepath.Base(path)
						// Check to see if the resource name matches the one of the ones passed on the -name flag
						if fname == name {
							zipResource(path, "")
						}
					}
				}
				return nil
			}
		}

		// These should be file resources, but, could be child folders of unzipped resources in
		// which case we will have handled them above.
		if (filepath.Dir(path) != metaFolder && !f.IsDir()) || (f.Name() == "aura" || f.Name() == "lwc") {
			return nil
		}
		// Again, if no names where specifed on -name flag, just add the file.
		if len(metadataName) == 0 {
			files = append(files, path)
		} else {
			// iterate the -name flag values looking for the ones specified
			for _, name := range metadataName {
				// Check if the file matches the metadata named.  For example, for
				// custom objects, the Account.object file matches the metadata
				// name Account.  For metadata types stored with separate -meta.xml
				// files, both files should match, e.g. HelloWorld.cls and
				// HelloWorld.cls-meta.xml.  For custom metadata named
				// My_Type.My_Object, the file My_Type.My_Object.md will match.
				if FilenameMatchesMetadataName(filepath.Base(path), name) {
					files = append(files, path)
				}
			}
		}

		return nil
	})

	// Push these files to the package maker/sender
	PushByPaths(force, files, true, namePaths, &options)
}

// Just zip up what ever is in the path
func zipResource(path string, topLevelFolder string) {
	zipfile := new(bytes.Buffer)
	zipper := zip.NewWriter(zipfile)
	startPath := path + "/"
	filepath.Walk(path, func(path string, f os.FileInfo, err error) error {
		if strings.ToLower(filepath.Base(path)) != ".ds_store" {
			// Can skip dirs since the dirs will be created when the files are added
			if !f.IsDir() {
				file, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}
				fl, err := zipper.Create(filepath.Join(topLevelFolder, strings.Replace(path, startPath, "", -1)))
				if err != nil {
					ErrorAndExit(err.Error())
				}
				_, err = fl.Write([]byte(file))
				if err != nil {
					ErrorAndExit(err.Error())
				}
			}
		}
		return nil
	})

	zipper.Close()
	zipdata := zipfile.Bytes()
	ioutil.WriteFile(path+".resource", zipdata, 0644)
	return
}

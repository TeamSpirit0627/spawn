package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/spf13/cobra"
	"github.com/strangelove-ventures/simapp"
)

type SpawnNewConfig struct {
	ProjectName  string
	Bech32Prefix string
	AppName      string
	AppDirName   string
	BinaryName   string
	TokenDenom   string

	IgnoreFiles []string

	Debugging bool

	DisabledFeatures []string
}

const (
	FlagWalletPrefix = "bech32"
	FlagBinaryName   = "bin"
	FlagDebugging    = "debug"
	FlagTokenDenom   = "denom"

	FlagDisabled = "disable"
)

var (
	IgnoredFiles      = []string{"generate.sh", "embed.go"}
	SupportedFeatures = []string{"tokenfactory", "poa", "globalfee", "wasm", "ibc", "nft", "group", "circuit"}
)

func init() {
	newChain.Flags().String(FlagWalletPrefix, "cosmos", "chain wallet bech32 prefix")
	newChain.Flags().String(FlagBinaryName, "appd", "binary name")
	newChain.Flags().Bool(FlagDebugging, false, "enable debugging")
	newChain.Flags().StringSlice(FlagDisabled, []string{}, "disable features: "+strings.Join(SupportedFeatures, ", "))
	newChain.Flags().String(FlagTokenDenom, "stake", "token denom")
}

// TODO: reduce required inputs here. (or make them flags with defaults?)
var newChain = &cobra.Command{
	Use:     "new [project-name]",
	Short:   "List all current chains or outputs a current config information",
	Example: fmt.Sprintf(`spawn new project --%s=cosmos --%s=appd`, FlagWalletPrefix, FlagBinaryName),
	Args:    cobra.ExactArgs(1),
	// ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// 	return GetFiles(), cobra.ShellCompDirectiveNoFileComp
	// },
	Run: func(cmd *cobra.Command, args []string) {
		projName := strings.ToLower(args[0])
		appName := strings.Title(projName) + "App"

		walletPrefix, _ := cmd.Flags().GetString(FlagWalletPrefix)
		binName, _ := cmd.Flags().GetString(FlagBinaryName)
		denom, _ := cmd.Flags().GetString(FlagTokenDenom)

		debug, _ := cmd.Flags().GetBool(FlagDebugging)

		disabled, _ := cmd.Flags().GetStringSlice(FlagDisabled)

		cfg := SpawnNewConfig{
			ProjectName:  projName,
			Bech32Prefix: walletPrefix,
			AppName:      appName,
			AppDirName:   "." + projName,
			BinaryName:   binName,
			TokenDenom:   denom,
			Debugging:    debug,

			// by default everything is on, then we remove what the user wants to disable
			DisabledFeatures: disabled,
		}

		NewChain(cfg)

	},
}

func NewChain(cfg SpawnNewConfig) {
	NewDirName := cfg.ProjectName
	bech32Prefix := cfg.Bech32Prefix
	appName := cfg.AppName
	appDirName := cfg.AppDirName
	binaryName := cfg.BinaryName
	Debugging := cfg.Debugging
	disabled := cfg.DisabledFeatures

	fmt.Println("Disabled features:", disabled)

	goModName := fmt.Sprintf("github.com/strangelove-ventures/%s", NewDirName)

	fmt.Println("Spawning new app:", NewDirName)

	if err := os.MkdirAll(NewDirName, 0755); err != nil {
		panic(err)
	}

	err := fs.WalkDir(simapp.SimApp, ".", func(relPath string, d fs.DirEntry, e error) error {
		newPath := path.Join(NewDirName, relPath)

		// if Debugging {
		// 	fmt.Printf("relPath: %s, newPath: %s\n", relPath, newPath)
		// }

		if relPath == "." {
			return nil
		}

		if d.IsDir() {
			// if relPath is a dir, continue walking
			return nil
		}

		for _, ignoreFile := range IgnoredFiles {
			if strings.HasSuffix(newPath, ignoreFile) {
				if Debugging {
					fmt.Println("ignoring", newPath)
				}
				return nil
			}
		}

		// grab the file contents from path
		fileContent, err := simapp.SimApp.ReadFile(relPath)
		if err != nil {
			return err
		}
		fileContent = removeDisabledFeatures(disabled, relPath, fileContent)

		fc := string(fileContent)

		if fc == "REMOVE" {
			// don't save this file
			return nil
		}

		if relPath == "scripts/test_node.sh" {
			fc = strings.ReplaceAll(fc, "export BINARY=${BINARY:-wasmd}", fmt.Sprintf("export BINARY=${BINARY:-%s}", binaryName))
			fc = strings.ReplaceAll(fc, "export DENOM=${DENOM:-token}", fmt.Sprintf("export DENOM=${DENOM:-%s}", cfg.TokenDenom))
		}

		// TODO: regex would be nicer for replacing incase it changes up stream. may never though. Also limit to specific files?
		fc = strings.ReplaceAll(fc, ".wasmd", appDirName)
		fc = strings.ReplaceAll(fc, `const appName = "WasmApp"`, fmt.Sprintf(`const appName = "%s"`, appName))
		fc = strings.ReplaceAll(fc, `Bech32Prefix = "wasm"`, fmt.Sprintf(`Bech32Prefix = "%s"`, bech32Prefix))
		fc = strings.ReplaceAll(fc, "github.com/strangelove-ventures/simapp", goModName)

		// MakeFileReplace
		fc = strings.ReplaceAll(fc, "https://github.com/CosmWasm/wasmd.git", fmt.Sprintf("https://%s.git", goModName))
		fc = strings.ReplaceAll(fc, "version.Name=wasm", fmt.Sprintf("version.Name=%s", appName)) // ldflags
		fc = strings.ReplaceAll(fc, "version.AppName=wasmd", fmt.Sprintf("version.AppName=%s", binaryName))
		fc = strings.ReplaceAll(fc, "github.com/CosmWasm/wasmd/app.Bech32Prefix=wasm", fmt.Sprintf("%s/app.Bech32Prefix=%s", goModName, bech32Prefix))
		fc = strings.ReplaceAll(fc, "cmd/wasmd", fmt.Sprintf("cmd/%s", binaryName))
		fc = strings.ReplaceAll(fc, "build/wasmd", fmt.Sprintf("build/%s", binaryName))

		// heighliner
		if strings.HasSuffix(relPath, "chains.yaml") {
			fc = strings.ReplaceAll(fc, "MyAppName", appName)
			fc = strings.ReplaceAll(fc, "MyAppBinary", binaryName)
		}
		fc = strings.ReplaceAll(fc, "heighliner build -c juno --local -f ./chains.yaml", fmt.Sprintf(`heighliner build -c %s --local -f ./chains.yaml`, strings.ToLower(appName)))

		// if the relPath is cmd/wasmd, replace it to be cmd/binaryName
		if strings.HasPrefix(relPath, "cmd/wasmd") {
			newPath = strings.ReplaceAll(newPath, "cmd/wasmd", fmt.Sprintf("cmd/%s", binaryName))
		}

		if err := os.MkdirAll(path.Dir(newPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(newPath, []byte(fc), 0644); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
}

// Removes disabled features from the files specified
func removeDisabledFeatures(disabled []string, relativePath string, fileContent []byte) []byte {
	for _, name := range disabled {
		switch name {
		case "tokenfactory":
			fileContent = removeTokenFactory(relativePath, fileContent)
		case "poa":
			fileContent = removePoa(relativePath, fileContent)
		case "globalfee":
			fileContent = removeGlobalFee(relativePath, fileContent)
		case "ibc": // this would remove all. Including PFM, then we can have others for specifics (i.e. ICAHost, IBCFees)
			// fileContent = removeIbc(relativePath, fileContent)
			continue
		case "wasm":
			fileContent = removeWasm(relativePath, fileContent)
			continue
		case "nft":
			// fileContent = removeNft(relativePath, fileContent)
			continue
		case "circuit":
			// fileContent = removeCircuit(relativePath, fileContent)
			continue
		}
	}

	// remove any left over `// spawntag:` comments
	fileContent = RemoveTaggedLines("", string(fileContent), false)

	return fileContent
}

// Removes all references from the tokenfactory file
func removeTokenFactory(relativePath string, fileContent []byte) []byte {
	if relativePath == "go.mod" || relativePath == "go.sum" {
		fileContent = RemoveGoModImport("github.com/reecepbcups/tokenfactory", fileContent)
	}

	if relativePath == "app/app.go" {
		fileContent = RemoveGeneralModule("tokenfactory", string(fileContent))
	}

	if relativePath == "scripts/test_node.sh" {
		fileContent = RemoveGeneralModule("tokenfactory", string(fileContent))
	}

	return fileContent
}

func removePoa(relativePath string, fileContent []byte) []byte {
	if relativePath == "go.mod" || relativePath == "go.sum" {
		fileContent = RemoveGoModImport("github.com/strangelove-ventures/poa", fileContent)
	}

	if relativePath == "app/app.go" || relativePath == "app/ante.go" {
		fileContent = RemoveGeneralModule("poa", string(fileContent))
	}

	if relativePath == "scripts/test_node.sh" {
		fileContent = RemoveGeneralModule("poa", string(fileContent))
	}

	return fileContent
}

func removeGlobalFee(relativePath string, fileContent []byte) []byte {

	fileContent = HandleCommentSwaps("globalfee", string(fileContent))
	fileContent = RemoveTaggedLines("globalfee", string(fileContent), true)

	if relativePath == "go.mod" || relativePath == "go.sum" {
		fileContent = RemoveGoModImport("github.com/reecepbcups/globalfee", fileContent)
	}

	if relativePath == "app/app.go" || relativePath == "app/ante.go" {
		fileContent = RemoveGeneralModule("globalfee", string(fileContent))
		fileContent = RemoveGeneralModule("GlobalFee", string(fileContent))
	}

	if relativePath == "scripts/test_node.sh" {
		fileContent = RemoveGeneralModule("globalfee", string(fileContent))
	}

	return fileContent
}

func removeWasm(relativePath string, fileContent []byte) []byte {

	// remove any line with spawntag:wasm
	// if strings.Contains(string(fileContent), "spawntag:wasm") {}
	fileContent = RemoveTaggedLines("wasm", string(fileContent), true)

	// TODO: tokenfactory depends on wasm currently.
	if relativePath == "go.mod" || relativePath == "go.sum" {
		fileContent = RemoveGoModImport("github.com/CosmWasm/wasmd", fileContent)
		fileContent = RemoveGoModImport("github.com/CosmWasm/wasmvm", fileContent)
	}

	if relativePath == "app/app.go" || relativePath == "app/ante.go" {
		for _, w := range []string{
			"WasmKeeper", "wasmtypes", "wasmStack",
			"wasmOpts", "TXCounterStoreService", "WasmConfig",
			"wasmDir", "tokenfactorybindings", "github.com/CosmWasm/wasmd", "wasmvm",
		} {
			fileContent = RemoveGeneralModule(w, string(fileContent))
		}

	}

	if relativePath == "app/ante.go" {
		fileContent = RemoveGeneralModule("wasm", string(fileContent))
	}

	if relativePath == "app/encoding.go" {
		fileContent = RemoveGeneralModule("wasmkeeper", string(fileContent))
	}

	if relativePath == "app/sim_test.go" {
		fileContent = RemoveGeneralModule("wasm", string(fileContent))
	}

	if relativePath == "app/app_test.go" {
		fileContent = RemoveGeneralModule("wasmOpts", string(fileContent))
		fileContent = RemoveGeneralModule("wasmkeeper", string(fileContent))
	}

	if relativePath == "app/test_support.go" {
		fileContent = RemoveGeneralModule("wasm", string(fileContent))
	}

	if relativePath == "app/test_helpers.go" {
		for _, w := range []string{"emptyWasmOptions", "wasmkeeper", "WasmOpts", "wasmOpts"} {
			fileContent = RemoveGeneralModule(w, string(fileContent))
		}

	}

	if relativePath == "app/wasm.go" {
		fileContent = []byte("REMOVE")
	}

	if relativePath == "cmd/wasmd/commands.go" {
		for _, w := range []string{"wasm", "wasmOpts", "wasmcli", "wasmtypes"} {
			fileContent = RemoveGeneralModule(w, string(fileContent))
		}
	}

	if relativePath == "cmd/wasmd/root.go" {
		for _, w := range []string{"wasmtypes", "wasmkeeper"} {
			fileContent = RemoveGeneralModule(w, string(fileContent))
		}
	}

	return fileContent
}

// Sometimes if we remove a module, we want to delete one line and use another.
func HandleCommentSwaps(name string, fileContent string) []byte {
	newContent := make([]string, 0, len(strings.Split(fileContent, "\n")))

	uncomment := fmt.Sprintf("?spawntag:%s", name)

	for idx, line := range strings.Split(fileContent, "\n") {
		hasUncommentTag := strings.Contains(line, uncomment)
		if hasUncommentTag {
			line = strings.Replace(line, "//", "", 1)
			line = strings.TrimRight(strings.Replace(line, fmt.Sprintf("// %s", uncomment), "", 1), " ")
			fmt.Printf("uncomment %s: %d, %s\n", name, idx, line)
		}

		newContent = append(newContent, line)
	}

	return []byte(strings.Join(newContent, "\n"))
}

const expectedFormat = "// spawntag:"

// RemoveTaggedLines deletes tagged lines or just removes the comment if desired.
func RemoveTaggedLines(name string, fileContent string, deleteLine bool) []byte {
	newContent := make([]string, 0, len(strings.Split(fileContent, "\n")))

	startIdx := -1
	for idx, line := range strings.Split(fileContent, "\n") {
		// TODO: regex anything in between // and spawntag such as spaces, symbols, etc.
		line = strings.ReplaceAll(line, "//spawntag:", expectedFormat) // just QOL for us to not tear our hair out

		hasTag := strings.Contains(line, fmt.Sprintf("spawntag:%s", name))
		hasMultiLineTag := strings.Contains(line, fmt.Sprintf("!spawntag:%s", name))

		// if the line has a tag, and the tag starts with a !, then we will continue until we find the end of the tag with another.
		if startIdx != -1 {
			if !hasMultiLineTag {
				continue
			}

			startIdx = -1
			fmt.Println("endIdx:", idx, line)
			continue
		}

		if hasMultiLineTag {
			if !deleteLine {
				continue
			}

			startIdx = idx
			fmt.Printf("startIdx %s: %d, %s\n", name, idx, line)
			continue
		}

		if hasTag {
			if deleteLine {
				continue
			}

			line = strings.Split(line, expectedFormat)[0]
			line = strings.TrimRight(line, " ")
		}

		newContent = append(newContent, line)
	}

	return []byte(strings.Join(newContent, "\n"))
}

// RemoveGeneralModule removes any matching names from the fileContent.
// i.e. if moduleFind is "tokenfactory" any lines with "tokenfactory" will be removed
// including comments.
// If an import or other line depends on a solo module a user wishes to remove, add a comment to the line
// such as `// tag:tokenfactory` to also remove other lines within the simapp template
func RemoveGeneralModule(removeText string, fileContent string) []byte {
	newContent := make([]string, 0, len(strings.Split(fileContent, "\n")))

	startIdx := -1
	for idx, line := range strings.Split(fileContent, "\n") {
		// if we are in a startIdx, then we need to continue until we find the close parenthesis (i.e. NewKeeper)
		if startIdx != -1 {
			fmt.Printf("rm %s startIdx: %d, %s\n", removeText, idx, line)
			if strings.TrimSpace(line) == ")" || strings.TrimSpace(line) == "}" {
				fmt.Println("endIdx:", idx, line)
				startIdx = -1
				continue
			}

			continue
		}

		lineHas := strings.Contains(line, removeText)

		if lineHas && (strings.HasSuffix(strings.TrimSpace(line), "(") || strings.HasSuffix(strings.TrimSpace(line), "{")) {
			startIdx = idx
			fmt.Printf("startIdx %s: %d, %s\n", removeText, idx, line)
			continue
		}

		if lineHas {
			fmt.Printf("rm %s: %d, %s\n", removeText, idx, line)
			continue
		}

		newContent = append(newContent, line)
	}

	return []byte(strings.Join(newContent, "\n"))
}

// given a go mod, remove a line within the file content
func RemoveGoModImport(module string, fileContent []byte) []byte {
	fcs := string(fileContent)
	lines := strings.Split(fcs, "\n")

	newLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.Contains(line, module) {
			newLines = append(newLines, line)
		}
	}

	return []byte(strings.Join(newLines, "\n"))
}

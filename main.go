package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
)

type Function struct {
	Name     string
	Position string
	CalledBy []*Function
}

var mermaid bool

func init() {
	flag.BoolVar(&mermaid, "m", false, "Output in Mermaid format instead of JSON")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-m] position\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
}

func main() {
	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	position := flag.Args()[0]

	root, err := getFunctionDefinition(position)
	checkFatal(err)

	setCallerFunctions(root)

	if mermaid {
		printMermaidDiagram(root)
	} else {
		err := printJSON(root)
		checkFatal(err)
	}
}

func checkFatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func getFunctionDefinition(position string) (*Function, error) {
	out, err := runGopls("definition", position)
	if err != nil {
		return nil, err
	}

	definitionLine := bytes.Split(out, []byte{'\n'})[0]
	match := getRegexMatch(`^(.+:) defined here as (func .+)$`, definitionLine)
	if match == nil {
		return nil, fmt.Errorf("%s is not a function", position)
	}

	return &Function{
		Name:     string(match[2]),
		Position: string(match[1]),
		CalledBy: make([]*Function, 0, 8),
	}, nil
}

func setCallerFunctions(callee *Function) {
	out, err := runGopls("call_hierarchy", callee.Position)
	if err != nil {
		callee.CalledBy = append(callee.CalledBy, &Function{
			Name:     fmt.Sprintf("Error: %v", err),
			Position: "Error",
			CalledBy: nil,
		})
		return
	}

	lines := bytes.Split(out, []byte{'\n'})
	for _, line := range lines {
		match := getRegexMatch(`^caller\[\d+\]:.+function .+ in (.+)$`, line)
		if match == nil {
			continue
		}

		caller, err := getFunctionDefinition(string(match[1]))
		if err != nil {
			callee.CalledBy = append(callee.CalledBy, &Function{
				Name:     fmt.Sprintf("Error: %v", err),
				Position: "Error",
				CalledBy: nil,
			})
			return
		}

		callee.CalledBy = append(callee.CalledBy, caller)

		setCallerFunctions(caller)
	}
}

func runGopls(feature string, position string) ([]byte, error) {
	out, err := exec.Command("gopls", feature, position).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("`gopls %s` failed: %w: %s", feature, err, out)
	}
	return out, nil
}

func getRegexMatch(pattern string, data []byte) [][]byte {
	re := regexp.MustCompile(pattern)
	return re.FindSubmatch(data)
}

func printMermaidDiagram(root *Function) {
	fmt.Println("graph TD")

	visited := make(map[string]bool)
	printCaller(root, visited)

	fmt.Println("    style Error fill:#f00,stroke:#000,stroke-width:2px;")
}

func printCaller(f *Function, visited map[string]bool) {
	if visited[f.Position] {
		return
	}
	visited[f.Position] = true

	for _, caller := range f.CalledBy {
		fmt.Printf("    %s[%q]-->%s[%q]\n", caller.Position, caller.Name, f.Position, f.Name)

		printCaller(caller, visited)
	}
}

func printJSON(function *Function) error {
	marshalled, err := json.MarshalIndent(function, "", "\t")
	if err != nil {
		return err
	}

	fmt.Println(string(marshalled))
	return nil
}

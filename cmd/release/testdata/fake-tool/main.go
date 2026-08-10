package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var assets = []string{
	"jb_linux_amd64.tar.gz",
	"jb_linux_amd64.tar.gz.sha256",
	"jb_linux_arm64.tar.gz",
	"jb_linux_arm64.tar.gz.sha256",
	"jb_windows_amd64.zip",
	"jb_windows_amd64.zip.sha256",
	"jb_windows_arm64.zip",
	"jb_windows_arm64.zip.sha256",
}

func main() {
	tool := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	args := os.Args[1:]
	appendLog(tool + " " + strings.Join(args, " ") + "\n")
	switch tool {
	case "git":
		runGit(args)
	case "go":
		return
	case "pwsh":
		runPowerShell(args)
	case "gh":
		runGitHub(args)
	default:
		fail("unknown fake tool " + tool)
	}
}

func runGit(args []string) {
	joined := strings.Join(args, " ")
	switch joined {
	case "rev-parse --show-toplevel":
		fmt.Println(os.Getenv("JB_FAKE_ROOT"))
	case "symbolic-ref --short HEAD":
		fmt.Println("main")
	case "status --porcelain", "fetch origin main --tags", "tag -a v1.2.4 abc123 -m Release v1.2.4", "push origin refs/tags/v1.2.4":
		return
	case "remote get-url origin":
		fmt.Println("git@github.com:zarxor/cli.git")
	case "rev-parse HEAD", "rev-parse origin/main":
		fmt.Println("abc123")
	case "tag --list":
		fmt.Println("v1.2.3")
	default:
		fail("unexpected git command: " + joined)
	}
}

func runPowerShell(args []string) {
	for index, arg := range args {
		if arg == "-OutputDir" && index+1 < len(args) {
			dir := args[index+1]
			if err := os.MkdirAll(dir, 0o700); err != nil {
				fail(err.Error())
			}
			for _, name := range assets {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
					fail(err.Error())
				}
			}
			return
		}
	}
}

func runGitHub(args []string) {
	joined := strings.Join(args, " ")
	if joined == "auth status" || strings.HasPrefix(joined, "release create v1.2.4") {
		return
	}
	root := os.Getenv("JB_FAKE_ROOT")
	publishedMarker := filepath.Join(root, "published")
	if joined == "release edit v1.2.4 --draft=false" {
		if err := os.WriteFile(publishedMarker, nil, 0o600); err != nil {
			fail(err.Error())
		}
		return
	}
	if joined == "release view v1.2.4 --json isDraft,url,assets" {
		_, err := os.Stat(publishedMarker)
		draft := os.IsNotExist(err)
		response := struct {
			IsDraft bool   `json:"isDraft"`
			URL     string `json:"url"`
			Assets  []struct {
				Name string `json:"name"`
			} `json:"assets"`
		}{IsDraft: draft, URL: "https://github.com/zarxor/cli/releases/tag/v1.2.4"}
		for _, name := range assets {
			response.Assets = append(response.Assets, struct {
				Name string `json:"name"`
			}{Name: name})
		}
		if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
			fail(err.Error())
		}
		return
	}
	fail("unexpected gh command: " + joined)
}

func appendLog(contents string) {
	path := os.Getenv("JB_FAKE_LOG")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fail(err.Error())
	}
	defer file.Close()
	if _, err := file.WriteString(contents); err != nil {
		fail(err.Error())
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

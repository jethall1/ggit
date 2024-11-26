package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Usage: your_program.sh <command> <arg1> <arg2> ...
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
		for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
			}
		}

		headFileContents := []byte("ref: refs/heads/main\n")
		if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
		}

		fmt.Println("Initialized git directory")

	case "cat-file":
		objName := os.Args[3][2:]
		folderName := os.Args[3][:2]

		filePath := fmt.Sprintf(".git/objects/%s/%s", folderName, objName)
		byts, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Println("failed to read file")
		}

		buffer := bytes.NewBuffer(byts)
		zlibReader, zlibErr := zlib.NewReader(buffer)
		if zlibErr != nil {
			log.Fatal("error decoding zlib buffer")
		}

		blob := new(bytes.Buffer)
		_, err = io.Copy(blob, zlibReader)
		if err != nil {
			log.Fatal("failed to convert zlib buffer")
		}

		nStr := blob.String()
		content := nStr[strings.Index(nStr, "\x00")+len("\x00"):]

		fmt.Print(content)
	case "hash-object":
		objName := os.Args[3]
		filePath1 := fmt.Sprintf("./%s", objName)
		contents, err := os.Open(filePath1)
		if err != nil {
			fmt.Println("failed to read file")
		}

		blob := new(bytes.Buffer)
		_, err = io.Copy(blob, contents)
		if err != nil {
			log.Fatal("failed to convert zlib buffer")
		}

		sha := sha1.New()
		sha.Write(blob.Bytes())

		if os.Args[2] == "-w" {
			contentWithBlob := fmt.Sprintf("blob %d\x00%s", len(blob.String()), blob.String())

			sha := (sha1.Sum([]byte(contentWithBlob)))
			hash := fmt.Sprintf("%x", sha)

			folderPre := hash[:2]
			blobName := hash[2:]

			fileObjPath := fmt.Sprintf(".git/objects/%s/%s", folderPre, blobName)

			dirPath := filepath.Dir(fileObjPath)
			err := os.MkdirAll(dirPath, 0755)
			if err != nil {
				fmt.Println("Error creating directories:", err)
				return
			}

			// fmt.Println(blobName)
			f, err := os.Create(fileObjPath)
			if err != nil {
				fmt.Println("error creating file", err)
				return
			}
			defer f.Close()

			// Create a zlib writer
			zlibWriter := zlib.NewWriter(f)
			defer zlibWriter.Close()

			// Write compressed data to the file
			_, err = zlibWriter.Write([]byte(contentWithBlob))
			if err != nil {
				fmt.Println("Error writing compressed data:", err)
				return
			}

			fmt.Println(hash)

		}

	case "ls-tree":
		if os.Args[2] == "--name-only" {
			hash := os.Args[3]
			folderPre := hash[:2]
			blobName := hash[2:]

			dirPath := fmt.Sprintf(".git/objects/%s", folderPre)

			byts, err := os.ReadFile(dirPath + "/" + blobName)
			if err != nil {
				fmt.Println("failed to read file")
			}

			buffer := bytes.NewBuffer(byts)
			zlibReader, zlibErr := zlib.NewReader(buffer)
			if zlibErr != nil {
				log.Fatal("error decoding zlib buffer")
			}

			blob := new(bytes.Buffer)
			_, err = io.Copy(blob, zlibReader)
			if err != nil {
				log.Fatal("failed to convert zlib buffer")
			}

			out := bytes.Split(blob.Bytes(), []byte("\x00"))
			use := out[1 : len(out)-1]
			for _, i := range use {
				splitter := []byte(" ")
				splitByte := bytes.Split(i, splitter)[1]
				fmt.Println(string(splitByte))
			}
		}
	case "write-tree":
		// treat ./ as staging area for sake of simplicity
		cwd, _ := os.Getwd()
		h, c := generateTree(cwd)
		treeHash := hex.EncodeToString(h)
		os.Mkdir(filepath.Join(".git", "objects", treeHash[:2]), 0755)

		var compressed bytes.Buffer
		w := zlib.NewWriter(&compressed)
		w.Write(c)
		w.Close()

		os.WriteFile(filepath.Join(".git", "objects", treeHash[:2], treeHash[2:]), compressed.Bytes(), 0644)
		fmt.Println(treeHash)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}

type treeObject struct {
	name   string
	hashes []byte
}

func generateTree(dir string) ([]byte, []byte) {
	var entries []treeObject
	contentSize := 0
	fileInfos, _ := os.ReadDir(dir)
	for _, fileInfo := range fileInfos {
		// this would ideally read from .gitignore
		// but that doesnt exist yet so this will do
		if fileInfo.Name() == ".git" || fileInfo.Name() == ".codecrafters" || fileInfo.Name() == ".gitattributes" {
			continue
		}
		fmt.Println("reading file ", fileInfo.Name())

		if !fileInfo.IsDir() {
			reader, _ := os.Open(filepath.Join(dir, fileInfo.Name()))
			hash := generateBlobHash(reader)

			// 100644 normal file
			// 040000 tree
			fileString := fmt.Sprintf("100644 %s\x00", fileInfo.Name())

			b := append([]byte(fileString), hash...)
			fmt.Println("array", string(b))

			entries = append(entries, treeObject{fileInfo.Name(), b})
			for _, i := range entries {
				fmt.Println("entry", string(i.hashes))
			}
			contentSize += len(b)
		} else {
			b, _ := generateTree(filepath.Join(dir, fileInfo.Name()))
			s := fmt.Sprintf("040000 %s\x00", fileInfo.Name())

			b2 := append([]byte(s), b...)
			entries = append(entries, treeObject{fileInfo.Name(), b2})
			contentSize += len(b2)
		}
	}

	s := fmt.Sprintf("tree %d\x00", contentSize)
	b := []byte(s)
	// fmt.Println(string(b))
	for _, entry := range entries {
		// fmt.Printf("%v\n", entry)
		b = append(b, entry.hashes...)
	}
	sha1 := sha1.New()
	// fmt.Println(string(b))
	io.WriteString(sha1, string(b))
	return sha1.Sum(nil), b
}

func generateBlobHash(fileReader io.Reader) string {
	blob := new(bytes.Buffer)
	_, err := io.Copy(blob, fileReader)
	if err != nil {
		log.Fatal("failed to convert zlib buffer")
	}

	sha := sha1.New()
	sha.Write(blob.Bytes())

	// this is just generating the hash for the file
	contentWithBlob := fmt.Sprintf("blob %d\x00%s", len(blob.String()), blob.String())
	shaOut := (sha1.Sum([]byte(contentWithBlob)))
	hash := fmt.Sprintf("%x", shaOut)

	return hash
}

package main

import (
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"time"
)

const NUM_FILES = 5
const MAX_NUM_CHAR_FILE = 1000
const MIN_NUM_CHAR_FILE = 100
const CHARACTER_SET = "abcdefghijklmnopqrstuvxyzw0123456789"

func GenerateRandomString(size int) []byte {
	res := make([]byte, size)
	characterSetLen := len(CHARACTER_SET)
	for i := 0; i < size; i++ {
		res[i] = CHARACTER_SET[rand.Intn(characterSetLen)]
	}

	return res
}

func GenerateFiles(path string) {
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < NUM_FILES; i++ {
		numCharacters := MIN_NUM_CHAR_FILE + rand.Intn(MAX_NUM_CHAR_FILE-MIN_NUM_CHAR_FILE)
		ioutil.WriteFile(path+"/"+strconv.FormatInt(int64(rand.Intn(100)+i), 10),
			GenerateRandomString(numCharacters), 0644)
	}
}

func main() {
	GenerateFiles(os.Args[1])
}

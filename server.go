package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	// "github.com/davecgh/go-spew/spew"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"unicode"
)

func isMn(r rune) bool {
	return unicode.Is(unicode.Mn, r) // Mn: nonspacing marks
}

var sh = shell.NewShell("localhost:5001")

func encrypt(key, text []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	b := base64.StdEncoding.EncodeToString(text)
	ciphertext := make([]byte, aes.BlockSize+len(b))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], []byte(b))
	return ciphertext, nil
}

func decrypt(key, text []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(text) < aes.BlockSize {
		return nil, errors.New("ciphertext too short")
	}
	iv := text[:aes.BlockSize]
	text = text[aes.BlockSize:]
	cfb := cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(text, text)
	data, err := base64.StdEncoding.DecodeString(string(text))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func cypher(words string) (mykey []byte, encode []byte) {
	key := make([]byte, 32)
	rand.Read(key)
	plaintext := []byte(words)
	ciphertext, err := encrypt(key, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	return key, ciphertext
}

type EncryptedPayload struct {
	Encrypted map[string]string
	Keys      map[string]string
}

type DecryptedPayload struct {
	Address string
	Keys    map[string]string
}

func addFileToIPFS(fileContents []byte) string {
	fc := string(fileContents)
	// spew.Dump(fc)
	cid, err := sh.Add(strings.NewReader(fc))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		os.Exit(1)
	}
	return cid
}

func getFileFromIPFS(fileName []byte) string {
	contents, _ := sh.ObjectGet(string(fileName))
	data := contents.Data
	return data
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	fmt.Println(`
 == Encryption + Decrpytion Server Started == 


 === Encrypt ================================

http://localhost:6256/encrypt

curl --request POST \
  --url http://localhost:6256/encrypt \
  --header 'content-type: application/json' \
  --data '{
	"text": "you cant see me!"
}'

 ============================================



 === Decrypt ================================

 http://localhost:6256/decrypt


curl --request POST \
  --url http://localhost:6256/decrypt \
  --header 'content-type: application/json' \
  --data '{
	"address":"QmZQZ4xdjAWminv28MuGBPAvofqkwquk9kUrdzFitxbcMt",
	"keys": {
		"text": "F/vY63lArqf2rr3ADAz2tE7mKmIRmRkzi3V5+ouHSIs="
	}
}'

 ============================================
`)

	r.POST("/encrypt", func(c *gin.Context) {
		val, _ := c.GetRawData() // get json from request
		str2 := string(val)
		var y map[string]interface{}
		json.Unmarshal([]byte(str2), &y)
		encryptedKeys := map[string]string{}
		encrpytedValues := map[string]string{}
		for k, v := range y {
			key, enc := cypher(v.(string))
			encryptedKeys[k] = base64.StdEncoding.EncodeToString(key)
			encrpytedValues[k] = base64.StdEncoding.EncodeToString(enc)
		}
		jsonString, _ := json.Marshal(encrpytedValues)
		addr := addFileToIPFS(jsonString)
		rankingsJson, _ := json.Marshal(encryptedKeys)
		ioutil.WriteFile("keyfiles/"+addr+".json", rankingsJson, 0644)
		c.JSON(200, gin.H{
			"message": "/ipfs/" + addr,
		})
	})

	r.POST("/decrypt", func(c *gin.Context) {
		val, _ := c.GetRawData()                      // get json from request
		var dpay DecryptedPayload                     // whats out decryption request object gonna look like
		json.Unmarshal(val, &dpay)                    // convert from string req to GO struct
		data := getFileFromIPFS([]byte(dpay.Address)) // fetch file from IPFS
		var parsedFileFromIPFS map[string]interface{}
		parse := strings.TrimFunc(data, func(r rune) bool {
			return r != '{' && r != '}'
		}) // we fix weird chars at start and end of obj - no idea what this is\
		json.Unmarshal([]byte(parse), &parsedFileFromIPFS)
		// spew.Dump(parsedFileFromIPFS)

		decryptedMessageMap := map[string]string{}

		disFailString := "THIS MESSAGE COULD NOT BE DECRYPTED"
		for k, mykey := range dpay.Keys { // loop over key value pairs
			value := parsedFileFromIPFS[k].(string)
			realKey, _ := base64.StdEncoding.DecodeString(mykey)
			realVal, _ := base64.StdEncoding.DecodeString(value)
			result, err := decrypt(realKey, realVal)
			if err != nil {
				// log.Fatal(err)
				fmt.Println("parse fail")
				fmt.Println(err)
				decryptedMessageMap[k] = disFailString
			} else {
				decryptedMessageMap[k] = string(result)
			}

		}
		// spew.Dump(decryptedMessageMap)
		c.JSON(200, gin.H{
			"message": decryptedMessageMap,
		})
	})
	r.Run(":6256") // listen and serve on 0.0.0.0:6256
}

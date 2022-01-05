// Simple CLI interface to Electronic Safe v2
//    https://bdsm.spuddy.org/writings/Safe_v2/
// designed for Emlalock to set a random password
// and then embed it into an image.
// This can now be the unlock image.
//
// Commands:
//  ./picture_lock {common} -lock -source source_image.jpg locked_image.jpg
//  ./picture_lock {common} -test locked_image.jpg
//  ./picture_lock {common} -unlock locked_image.jpg
//  ./picture_lock {common} -status
//
// Common options:
//  [-user username -pass password] -safe safe.name
//
// These can also be set in $HOME/.picture_lock (or %HOMEDIR%%HOMEPATH%
// on windows as a JSON file so they don't need to be passed each time
//
// e.g.
// {
// 	"Safe": "safe.local",
// 	"User": "username",
// 	"Pass": "password"
// }
//
// A safe name is mandatory, username/password are optional but if the
// safe requires them then you need to specify them

package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/tkanos/gonfig"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// What characters we allow for safe passwords.  In theory anything except
// a : should work, but we're gonna be more restrictive
const pswdstring = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// Information we read from the config file
type Configuration struct {
	Safe string
	User string
	Pass string
}

var configuration Configuration

// Make these global so they're easy to use, rather than passing them through
// a chain of main->{function}->talk_to_safe
var username, passwd, safe string

//////////////////////////////////////////////////////////////////////
//
// JPEG file handling
//
//////////////////////////////////////////////////////////////////////

// These are the fields we care about keeping
type JPEG struct {
	dqt      [10][]byte
	comment  []byte
	sof0     []byte
	dht      [10][]byte
	sos      []byte
	img      []byte
	dqtcount int
	dhtcount int
}

var lock_image JPEG

func read_jpeg_segment(img []byte, offset int) (int, int, []byte, error) {
	var segment int
	var size int
	var res []byte
	if img[offset] != 0xff {
		return 0, 0, nil, errors.New("Bad JPEG - expected 0xff at " + strconv.Itoa(offset))
	}
	segment = int(img[offset+1])
	size = int(img[offset+2])*256 + int(img[offset+3])
	res = img[offset+4 : offset+4+size-2]
	return segment, size, res, nil
}

func write_jpeg_segment(f io.Writer, marker int, data []byte) {
	var buf [4]byte
	l := len(data) + 2
	buf[0] = 0xff
	buf[1] = byte(marker)
	buf[2] = byte(l >> 8)
	buf[3] = byte(l & 255)
	f.Write(buf[:4])
	f.Write(data)
}

func parse_jpeg(img []byte) (JPEG, error) {
	var image JPEG

	if img[0] != 0xff && img[1] != 0xd8 {
		return image, errors.New("Image is not a JPEG - bad header")
	}
	if img[len(img)-2] != 0xff && img[len(img)-1] != 0xd9 {
		return image, errors.New("Image is not a JPEG - bad footer")
	}
	offset := 2

	for {
		section, size, data, err := read_jpeg_segment(img, offset)
		if err != nil {
			return image, err
		}
		offset += size + 2
		if section == 0xfe {
			image.comment = data
		} else if section == 0xc0 {
			image.sof0 = data
		} else if section == 0xda {
			image.sos = data
			break
		} else if section == 0xdb {
			image.dqt[image.dqtcount] = data
			image.dqtcount++
			if image.dqtcount > 9 {
				return image, errors.New("Too many DQT segments")
			}
		} else if section == 0xc4 {
			image.dht[image.dhtcount] = data
			image.dhtcount++
			if image.dhtcount > 9 {
				return image, errors.New("Too many DHT segments")
			}
		}
	}
	image.img = img[offset : len(img)-2]

	return image, nil
}

func read_jpeg(filename string) (JPEG, error) {
	img, err := ioutil.ReadFile(filename)
	if err != nil {
		return lock_image, errors.New("Could not open file " + filename)
	}
	return parse_jpeg(img)
}

func write_jpeg(f io.Writer, image JPEG) {
	var head [2]byte
	head[0] = 0xff
	head[1] = 0xd8
	var foot [2]byte
	foot[0] = 0xff
	foot[1] = 0xd9

	f.Write(head[:2])
	write_jpeg_segment(f, 0xfe, image.comment)
	for i := 0; i < image.dqtcount; i++ {
		write_jpeg_segment(f, 0xdb, image.dqt[i])
	}
	write_jpeg_segment(f, 0xc0, image.sof0)
	for i := 0; i < image.dhtcount; i++ {
		write_jpeg_segment(f, 0xc4, image.dht[i])
	}
	write_jpeg_segment(f, 0xda, image.sos)
	f.Write(image.img)
	f.Write(foot[:2])
}

//////////////////////////////////////////////////////////////////////
//
// Utility functions
//
//////////////////////////////////////////////////////////////////////

func abort(str string) {
	fmt.Fprintln(os.Stderr, "\n"+str)
	os.Exit(-1)
}

// Where do config files live?
func UserHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home + "\\"
	}
	return os.Getenv("HOME") + "/"
}

//////////////////////////////////////////////////////////////////////
//
// Talk to Safe
//
//////////////////////////////////////////////////////////////////////

func talk_to_safe(cmd string) string {
	url := "http://" + safe + "/safe/?" + cmd
	// fmt.Println("We want to to " + url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		// Ensure error doesn't have cmd in it...
		msg := strings.Replace(err.Error(), cmd, "*******", 1)
		abort("Got error setting up http request: " + msg)
	}

	req.SetBasicAuth(username, passwd)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		msg := strings.Replace(err.Error(), cmd, "*******", 1)
		abort("Problems talking to the safe: " + msg)
	}

	// Get the response as a string
	//   http://dlintw.github.io/gobyexample/public/http-client.html
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		abort("Problems getting response from safe: " + err.Error())
	}
	res := string(body)

	if resp.StatusCode != 200 {
		abort("Bad result from safe: " + resp.Status + "\n" + res)
	}
	return res
}

//////////////////////////////////////////////////////////////////////
//
// Main functions
//
//////////////////////////////////////////////////////////////////////

func lock(src, dest string) {
	if src == "" {
		abort("Missing --source file")
	}

	if src == dest {
		abort("Source and destination names can not be the same")
	}

	fmt.Println("Creating a new lock")
	lock_image, err := read_jpeg(src)
	if err != nil {
		abort(err.Error())
	}

	// Generate a random password
	b := make([]byte, 30)
	for i := range b {
		b[i] = pswdstring[rand.Intn(len(pswdstring))]
	}
	new_pswd := string(b)
	// DEBUG
	// new_pswd = "hello"

	// Lock the safe
	res := talk_to_safe("lock=1&lock1=" + new_pswd + "&lock2=" + new_pswd)
	if res != "Safe locked" {
		abort("Problem locking safe: " + res)
	}

	// Check the password was accepted
	res = talk_to_safe("pwtest=1&unlock=" + new_pswd)
	if res != "Passwords match" {
		abort("Unable to verify lock worked: " + res)
	}

	// Now embed the password in the image
	lock_image.comment = []byte("LOCKPSW:" + new_pswd)

	// Save the new image
	f, err := os.Create(dest)
	if err != nil {
		talk_to_safe("unlock_all=1&unlock=" + new_pswd)
		abort("We could not create the image file.  We have attempted to unlock the safe\nJust in case there was a problem the password generated was\n  " + new_pswd + "\nThe failure was: " + err.Error())
	}
	write_jpeg(f, lock_image)
	f.Close()
	fmt.Println(dest + " created.")
}

func unlock(file string, tst bool) {
	lock_image, err := read_jpeg(file)
	if err != nil {
		abort(err.Error())
	}

	psw := string(lock_image.comment)

	if strings.HasPrefix(psw, "LOCKPSW:") {
		psw = psw[8:]
	} else {
		abort("This is not a valid password image")
	}

	cmd := "unlock_all"
	if tst {
		cmd = "pwtest"
	}

	fmt.Println(talk_to_safe(cmd + "=1&unlock=" + psw))
}

func main() {
	// Let's seed our random function
	rand.Seed(time.Now().UnixNano())

	// Try and find the config file
	config_file := UserHomeDir() + ".picture_lock"
	if _, err := os.Stat(config_file); err == nil {
		// fmt.Println("Using configuration file " + config_file)

		parse := gonfig.GetConf(config_file, &configuration)
		if parse != nil {
			abort("Error parsing " + config_file + ": " + parse.Error())
		}
	}

	flag.StringVar(&username, "user", "", "Username to talk to safe (optional)")
	flag.StringVar(&passwd, "pass", "", "Password to talk to safe (optional)")
	flag.StringVar(&safe, "safe", "", "Safe Address")

	source := flag.String("source", "", "Source Image (needed for -lock)")
	lockflag := flag.Bool("lock", false, "Lock the safe, create new image")
	unlockflag := flag.Bool("unlock", false, "Unlock the safe with image")
	testflag := flag.Bool("test", false, "Test the image can unlock the safe")
	statusflag := flag.Bool("status", false, "Request current safe status")

	flag.Parse()

	// If the user didn't define these three things, use values
	// from the config file
	if username == "" {
		username = configuration.User
	}

	if passwd == "" {
		passwd = configuration.Pass
	}

	if safe == "" {
		safe = configuration.Safe
	}

	// Safe better be defined!
	if safe == "" {
		abort("No safe name passed")
	}

	if *statusflag {
		fmt.Println(talk_to_safe("status=1"))
		os.Exit(0)
	}

	args := flag.Args()

	if len(args) == 0 {
		abort("Missing filename; use the -h option for help")
	} else if len(args) != 1 {
		abort("Only one filename is allowed and must be the last value;\n  use the \"-h\" option for help")
	}

	filename := args[0]

	if *lockflag {
		lock(*source, filename)
	} else if *unlockflag {
		unlock(filename, false)
	} else if *testflag {
		unlock(filename, true)
	} else {
		abort("Command should be -lock or -unlock or -test; use -h for help")
	}
}

package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"
	"unicode"

	"github.com/fatih/color"
	"github.com/ncw/ncwtester/cwgenerator"
	"github.com/ncw/ncwtester/cwplayer"
	"golang.org/x/term"
)

var (
	sampleRate = flag.Int("samplerate", 44100, "sample rate")
	wpm        = flag.Float64("wpm", 25.0, "WPM to send at")
	frequency  = flag.Float64("frequency", 600.0, "HZ of morse")
	logFile    = flag.String("log", "ncwtesterstats.csv", "CSV file to log attempts")
	timeCutoff = flag.Duration("cutoff", 0, "If set, ignore stats older than this")
	letters    = flag.String("letters", "abcdefghijklmnopqrstuvwxyz0123456789.=/,?", "Letters to test")
	group      = flag.Int("group", 1, "Send letters in groups this big")
)

const (
	channelNum      = 2
	bitDepthInBytes = 2
	maxSampleValue  = 32767
)

func shuffleString(s string) string {
	rs := []rune(s)
	rand.Shuffle(len(rs), func(i, j int) {
		rs[i], rs[j] = rs[j], rs[i]
	})
	return string(rs)
}

// Returns whether the character is an exit character, eg CTRL-C or ESC
func isExit(r rune) bool {
	return r == 0x03 || r == 0x1B
}

// Reads a single character from the terminal
func getChar() (r rune) {
	s, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Failed to MakeRaw: %v", err)
	}
	defer func() {
		err := term.Restore(int(os.Stdin.Fd()), s)
		if err != nil {
			log.Fatalf("Failed to Restore: %v", err)
		}
	}()
	var buf [1]byte
	n, err := os.Stdin.Read(buf[:])
	if err != nil {
		log.Fatalf("Failed to Read: %v", err)
	}
	if n != 1 {
		log.Fatalf("Didn't read exactly 1 character")
	}
	return unicode.ToLower(rune(buf[0]))
}

func yorn(prompt string) bool {
	fmt.Printf("%s (y/n)> ", prompt)
	var c rune
	for {
		c = getChar()
		if c == 'y' || c == 'n' {
			break
		} else if isExit(c) {
			fmt.Println("...bye\n")
			os.Exit(0)
		}
	}
	fmt.Println(string(c))
	return c == 'y'
}

// Convert a duration into milliseconds
func ms(t time.Duration) int64 {
	return t.Milliseconds()
}

func run() error {
	opt := cwgenerator.Options{
		WPM:             *wpm,
		Frequency:       *frequency,
		SampleRate:      *sampleRate,
		ChannelNum:      channelNum,
		BitDepthInBytes: bitDepthInBytes,
		MaxSampleValue:  maxSampleValue,
	}
	cw, err := cwplayer.New(opt)
	if err != nil {
		return fmt.Errorf("failed to make cw generator: %w", err)
	}

	csvLog := NewCSVLog(*logFile)
	sessionStats := NewStats()

outer:
	for {
		// Bulk up the letters
		var testLetters = shuffleString(*letters)
		for len(testLetters)+len(*letters) <= 50 {
			testLetters += shuffleString(*letters)
		}
		// Make sure they are an whole number of groups
		for {
			remainder := len(testLetters) % *group
			if remainder == 0 {
				break
			}
			testLetters += string((*letters)[rand.Intn(len(*letters))])
		}
		if !yorn(fmt.Sprintf("Start test round with %d letters and %d groups?", len(testLetters), len(testLetters) / *group)) {
			break outer
		}

		cw.Reset()
		cw.String(" vvv   ")
		cw.SyncPlay()

		roundStats := NewStats()

		for i, tx := range testLetters {
			// Send all the letters at the start of the group
			if i%*group == 0 {
				cw.Reset()
				cw.Clear()
				cw.Rune(' ')
				for j := i; j < i+*group; j++ {
					cw.Rune(rune(testLetters[j]))
				}
				// cwDuration := cw.duration()
				// startPlaying := time.Now()
				cw.SyncPlay()
			}
			finishedPlaying := time.Now()
			// fmt.Printf("time to play %dms, expected %dms, diff=%dms\n", ms(finishedPlaying.Sub(startPlaying)), ms(cwDuration), ms(finishedPlaying.Sub(startPlaying)-cwDuration))

			rx := getChar()
			if isExit(rx) {
				break outer
			}
			reactionTime := time.Since(finishedPlaying)
			ok := rx == tx
			fmt.Printf("%2d/%2d: %c: reaction time %5dms: ", i+1, len(testLetters), tx, ms(reactionTime))
			if ok {
				color.Green("OK\n")
			} else {
				color.Red(fmt.Sprintf("BAD %c\n", rx))
			}
			csvLog.Add(tx, rx, reactionTime)
			roundStats.Add(string(tx), string(rx), reactionTime.Seconds())
			sessionStats.Add(string(tx), string(rx), reactionTime.Seconds())
		}

		fmt.Println("Round stats")
		roundStats.TotalSummary()
	}
	fmt.Println("Session stats")
	sessionStats.TotalSummary()

	stats := NewStats()
	stats.Load(*logFile, *timeCutoff)
	stats.Summary()

	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	flag.Parse()
	if err := run(); err != nil {
		panic(err)
	}
}

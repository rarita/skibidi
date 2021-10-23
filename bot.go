package main

import (
	"encoding/binary"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/kelseyhightower/envconfig"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type SkibidConfig struct {
	Token           string
	AllowedChannels []string
	SoundMaps       map[string]string
	GuildId         string
	VoiceChanId     string
	GracePlayPeriod int
}

type AudioStateLock struct {
	UnlockRequested bool
	internalMutex   sync.Mutex
}

func (l *AudioStateLock) Lock() {
	l.UnlockRequested = true
	l.internalMutex.Lock()
	l.UnlockRequested = false
}

func (l *AudioStateLock) Unlock() {
	l.internalMutex.Unlock()
}

const CfgPrefix = "skibid"
const DataPathPrefix = "data/"

var EnvCfg SkibidConfig
var soundBoard = make(map[string][][]byte, 8)

var audioStateLock = AudioStateLock{}

// misc. utils
func _sliceContains(needle *string, haystack *[]string) bool {
	for _, item := range *haystack {
		if item == *needle {
			return true
		}
	}

	return false
}

// loadSound attempts to load an encoded sound file from disk.
func loadSound(soundName *string) ([][]byte, error) {

	// check if soundboard already has this sound
	_, present := soundBoard[*soundName]
	if present {
		// dont do anything else
		return soundBoard[*soundName], nil
	}

	file, err := os.Open(fmt.Sprintf("%s%s.dca", DataPathPrefix, *soundName))
	defer file.Close()

	if err != nil {
		fmt.Println("Error opening dca sound file :", err)
		delete(soundBoard, *soundName)
		return nil, err
	}

	var opusLen int16

	// init array for key
	soundBoard[*soundName] = make([][]byte, 127)
	for {
		// Read opus frame length from dca file.
		err = binary.Read(file, binary.LittleEndian, &opusLen)
		if opusLen <= 0 {
			return nil, fmt.Errorf("invalid opus length %d", opusLen)
		}

		// If this is the end of the file, just return.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err := file.Close()
			if err != nil {
				delete(soundBoard, *soundName)
				return nil, err
			}
			return soundBoard[*soundName], nil
		}

		if err != nil {
			log.Println("Error reading from dca file :", err)
			delete(soundBoard, *soundName)
			return nil, err
		}

		// Read encoded pcm from dca file.
		InBuf := make([]byte, opusLen)
		err = binary.Read(file, binary.LittleEndian, &InBuf)

		// Should not be any end of file errors
		if err != nil {
			log.Println("Error reading from dca file :", err)
			delete(soundBoard, *soundName)
			return nil, err
		}

		// Append encoded pcm data to the buffer.
		soundBoard[*soundName] = append(soundBoard[*soundName], InBuf)
		// buffer = append(buffer, InBuf)
	}

}

// playSound plays the current buffer to the provided channel.
// should interrupt sound currently playing if another request
// was received.
func playSound(s *discordgo.Session, guildID, channelID, soundName string) (err error) {

	threadId := time.Now().Nanosecond()

	// Join the provided voice channel.
	// situation when bot already in channel is handled well
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}
	/*
		if err != nil {
			if _, ok := s.VoiceConnections[guildID]; ok {
				vc = s.VoiceConnections[guildID]
			} else {
				return err
			}
		}
	*/

	sound, err := loadSound(&soundName)
	if err != nil {
		log.Printf("Could not load sound %s", soundName)
		return err
	}

	// Try to acquire lock to play audio
	log.Printf("[%d] Trying to acquire audio state lock for %s...", threadId, soundName)
	audioStateLock.Lock()
	log.Printf("[%d] Acquired audio state lock for %s", threadId, soundName)
	defer audioStateLock.Unlock()

	// Start speaking and defer end speaking
	_ = vc.Speaking(true)
	defer vc.Speaking(false)

	// Send the buffer data.
	playPeriod := 0
	for _, buff := range sound {

		if len(buff) == 0 {
			continue
		}

		// check for mutex
		if playPeriod >= EnvCfg.GracePlayPeriod &&
			audioStateLock.UnlockRequested {
			log.Printf("[%d] Audio interrupted by incoming request: %s", threadId, soundName)
			log.Printf("play period interrupted: %d", playPeriod)
			return nil
		}

		vc.OpusSend <- buff
		playPeriod += 1

	}

	log.Printf("[%d] Sent whole audio to discord voice: %s", threadId, soundName)
	log.Printf("play period whole: %d", playPeriod)
	return nil

}

func soundNamesForMessage(message *discordgo.MessageCreate) (res []string, present bool) {

	if strings.Count(message.Content, ":") < 2 {
		return []string{}, false
	}

	res = make([]string, 0)
	for _, emojiCode := range strings.Split(message.Content, ":") {
		if val, exists := EnvCfg.SoundMaps[emojiCode]; exists {
			res = append(res, val)
		}
	}

	present = len(res) > 0
	return

}

// event handlers
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// ignore if message sender is skibidi
	if m.Author.ID == s.State.User.ID {
		return
	}
	// if message belong not to allowed channel, return
	if !(_sliceContains(&m.ChannelID, &EnvCfg.AllowedChannels)) {
		return
	}

	log.Printf("Got message: %s", m.Message.Content)

	// check if message is in sound map
	soundNames, present := soundNamesForMessage(m)
	if !(present) {
		return
	}

	for _, soundName := range soundNames {
		_soundName := soundName
		go func() {
			err := playSound(s, EnvCfg.GuildId, EnvCfg.VoiceChanId, _soundName)
			if err != nil {
				log.Printf("Could not play sound %s: %s", _soundName, err)
			}
		}()
	}

	/*
		_, err = s.ChannelMessageSend(m.ChannelID, m.Message.Content)
		if err != nil {
			log.Printf("Could not send message %s to channel %s", m.Message.Content, m.ChannelID)
		}
	*/

}

func main() {

	// load config from env
	err := envconfig.Process(CfgPrefix, &EnvCfg)
	if err != nil {
		log.Fatalf("Could not read environment configs: %s\n", err)
	}

	discord, err := discordgo.New("Bot " + EnvCfg.Token)
	if err != nil {
		log.Fatalf("Could not initialize discord bot with reason: %s\n", err)
	}

	log.Printf("Bot was initialized! Setting handler hooks up...")
	discord.AddHandler(messageCreate)
	discord.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	log.Printf("Starting up skibidi bot...")
	err = discord.Open()
	if err != nil {
		log.Fatalf("Could not open discord bot with reason: %s", err)
	}

	// Wait here until CTRL-C or other term signal is received.
	log.Println("Bot is now running.  Press CTRL-C to exit.")
	// todo fix
	_, _ = discord.ChannelVoiceJoin(EnvCfg.GuildId, EnvCfg.VoiceChanId, false, true)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	err = discord.Close()
	if err != nil {
		log.Fatalf("Attempted to gracefully close discord bot but got: %s", err)
	}
	log.Printf("Application is going to close...")

}

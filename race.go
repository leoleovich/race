package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/pkg/errors"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

const road_width = 40
const road_lenght = 20
const car_width = 14
const car_lenght = 7
const result_width = 76
const max_players_in_top = 10

type Config struct {
	Log                 *log.Logger
	AcidPath, ScorePath string
}

type Point struct {
	X, Y int
}

type Player struct {
	Name  string
	Score int64
}

type GameData struct {
	Roads              [][]byte
	Car, Clear, Splash []byte
	Top                []Player
}

type RoundData struct {
	player                                   Player
	CarPosition, bombPosition, bonusPosition Point
	BombFactor, BonusFactor, Speed           int
	GameOver                                 []byte
}

func generateRoad(reverse bool) []byte {
	road := make([]byte, road_width*road_lenght)
	midline := reverse
	for row := 0; row < road_lenght; row++ {
		for column := 0; column < road_width; column++ {
			var symbol byte
			if column == 0 || column == road_width-2 {
				symbol = byte('|')
			} else if column == road_width-1 {
				symbol = byte('\n')
			} else if column == road_width/2-1 {
				if midline {
					symbol = byte('|')
				} else {
					symbol = byte(' ')
				}
				midline = !midline
			} else {
				symbol = byte(' ')
			}
			road[row*road_width+column] = symbol
		}

	}
	return road
}

func getAcid(conf *Config, fileName string) ([]byte, error) {
	fileStat, err := os.Stat(conf.AcidPath + "/" + fileName)
	if err != nil {
		conf.Log.Printf("Acid %s does not exist: %v\n", fileName, err)
		return []byte{}, err
	}

	acid := make([]byte, fileStat.Size())
	f, err := os.OpenFile(conf.AcidPath+"/"+fileName, os.O_RDONLY, os.ModePerm)
	if err != nil {
		conf.Log.Printf("Error while opening %s: %v\n", fileName, err)
		os.Exit(1)
	}
	defer f.Close()

	f.Read(acid)

	return acid, nil
}

func updatePosition(conn net.Conn, position *Point) {
	for {
		direction := make([]byte, 1)

		_, err := conn.Read(direction)
		if err != nil {
			return
		}

		switch direction[0] {
		case 68:
			// Left
			position.X--
		case 67:
			// Right
			position.X++
		case 65:
			// Up
			position.Y--
		case 66:
			// Down
			position.Y++
		}
	}
}

func updateScore(roundData *RoundData) {
	for {
		roundData.player.Score++
		time.Sleep(time.Duration(roundData.Speed) * time.Millisecond)
	}

}

func readName(conf *Config, conn net.Conn, gameData *GameData) (string, error) {
	conn.Write(gameData.Splash)
	io := bufio.NewReader(conn)
	name, err := io.ReadString('\n')
	if err != nil {
		conf.Log.Println("Error while name reading", err)
		return "", err
	}
	if name == "" {
		conf.Log.Println("Empty name")
		return "", errors.New("Empty name")
	}
	if len(name) > result_width/2 {
		conf.Log.Println("Too long name")
		return "", errors.New("Too long name")
	}
	return strings.Replace(name, "\n", "", -1), nil
}

func gameOver(conf *Config, conn net.Conn, roundData *RoundData, gameData *GameData) {

	conn.Write(gameData.Clear)

	// First we print current player result:
	// Score
	scoreStr := fmt.Sprintf("%d", roundData.player.Score)
	for i, char := range scoreStr {
		roundData.GameOver[i] = byte(char)
	}
	//:
	roundData.GameOver[result_width/2] = byte(':')
	//Name
	for i, char := range []byte(roundData.player.Name) {
		roundData.GameOver[result_width-1-len(roundData.player.Name)+i] = byte(char)
	}

	// Then we check on which place is current player
	inserted := false
	for i, player := range gameData.Top {
		if roundData.player.Score >= player.Score {
			// Insert new record
			gameData.Top = append(gameData.Top[:i], append([]Player{roundData.player}, gameData.Top[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		gameData.Top = append(gameData.Top, roundData.player)
	}

	// Remove slowest user if top is full
	if len(gameData.Top) >= max_players_in_top {
		gameData.Top = gameData.Top[:max_players_in_top]
	}

	//TOP
	copy(roundData.GameOver[1*result_width+result_width/2-1:2*result_width+result_width/2+2], []byte("TOP"))

	// Then print new top
	for place, player := range gameData.Top {
		// Score
		scoreStr := fmt.Sprintf("%d", player.Score)
		for i, char := range scoreStr {
			roundData.GameOver[(2+place)*result_width+i] = byte(char)
		}
		//:
		roundData.GameOver[(2+place)*result_width+result_width/2] = byte(':')
		//Name
		for i, char := range []byte(player.Name) {
			roundData.GameOver[(2+place)*result_width+result_width-1-len(player.Name)+i] = byte(char)
		}
	}

	conn.Write(roundData.GameOver)
	// We do not need to check for error because user should not care, but logs are written
	saveScore(conf, gameData)
}

func saveScore(conf *Config, gameData *GameData) error {
	b, err := json.Marshal(gameData.Top)
	if err != nil {
		conf.Log.Println(err)
		return err
	}

	// Being sure file was not there
	os.Remove(conf.ScorePath + "/score.json")
	scoreFile, err := os.OpenFile(conf.ScorePath+"/score.json", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		conf.Log.Println(err)
		return err
	}
	defer scoreFile.Close()

	scoreFile.Write(b)
	return nil
}

func checkComplexity(roundData *RoundData) {
	// Checking and updating complexity
	if roundData.player.Score >= 600 {
		roundData.BonusFactor = 100
		roundData.BombFactor = 1
		roundData.Speed = 80
	} else if roundData.player.Score >= 400 {
		roundData.BonusFactor = 10
		roundData.BombFactor = 5
	} else if roundData.player.Score >= 200 {
		roundData.BonusFactor = 5
		roundData.Speed = 100
	} else if roundData.player.Score > 50 {
		roundData.Speed = 150
	}

}

func checkPosition(conf *Config, conn net.Conn, roundData *RoundData, gameData *GameData) bool {
	if roundData.CarPosition.X < 1 || roundData.CarPosition.X > road_width-car_width-1 || roundData.CarPosition.Y < 1 || roundData.CarPosition.Y > road_lenght-car_lenght-1 {
		// Hit the wall
		gameOver(conf, conn, roundData, gameData)
		return false
	} else if roundData.CarPosition.X <= roundData.bombPosition.X && roundData.CarPosition.X+car_width-1 > roundData.bombPosition.X &&
		roundData.CarPosition.Y <= roundData.bombPosition.Y && roundData.CarPosition.Y+car_lenght-1 > roundData.bombPosition.Y {
		// Hit the bomb
		gameOver(conf, conn, roundData, gameData)
		return false
	} else if roundData.CarPosition.X <= roundData.bonusPosition.X && roundData.CarPosition.X+car_width-1 > roundData.bonusPosition.X &&
		roundData.CarPosition.Y <= roundData.bonusPosition.Y && roundData.CarPosition.Y+car_lenght-1 > roundData.bonusPosition.Y {
		// Get the bonus
		roundData.player.Score += 10
		roundData.bonusPosition = Point{road_width, road_lenght}
	}
	return true
}

func round(conf *Config, conn net.Conn, gameData *GameData) {
	defer conn.Close()

	roundData := RoundData{}
	roundData.CarPosition = Point{road_width/2 - car_width/2, road_lenght - car_lenght - 1}
	roundData.bombPosition = Point{road_width, road_lenght}
	roundData.bonusPosition = Point{road_width, road_lenght}
	roundData.GameOver, _ = getAcid(conf, "game_over.txt")

	roundData.BombFactor = 10
	roundData.BonusFactor = 10
	roundData.Speed = 200

	name, err := readName(conf, conn, gameData)
	if err != nil {
		return
	}
	roundData.player.Name = name
	go updateScore(&roundData)
	go updatePosition(conn, &roundData.CarPosition)

	for {
		if !checkPosition(conf, conn, &roundData, gameData) {
			return
		}

		for i := range gameData.Roads {
			data := make([]byte, len(gameData.Roads[i]))
			copy(data, gameData.Roads[i])

			// Moving cursor at the beginning
			_, err := conn.Write(gameData.Clear)
			if err != nil {
				return
			}

			// Checking and updating complexity
			checkComplexity(&roundData)

			// Applying the bonus
			if roundData.bonusPosition.Y < road_lenght {
				data[roundData.bonusPosition.Y*road_width+roundData.bonusPosition.X] = byte('$')
				roundData.bonusPosition.Y++
			} else if rand.Int()%roundData.BonusFactor == 0 {
				roundData.bonusPosition.X, roundData.bonusPosition.Y = rand.Intn(road_width-3)+1, 0
			}

			// Applying the bomb
			if roundData.bombPosition.Y < road_lenght {
				data[roundData.bombPosition.Y*road_width+roundData.bombPosition.X] = byte('X')
				roundData.bombPosition.Y++
			} else if rand.Int()%roundData.BombFactor == 0 {
				roundData.bombPosition.X, roundData.bombPosition.Y = rand.Intn(road_width-3)+1, 0
			}

			// Applying the Car
			for line := 0; line < car_lenght; line++ {
				copy(data[((roundData.CarPosition.Y+line)*road_width+roundData.CarPosition.X):((roundData.CarPosition.Y+line)*road_width+roundData.CarPosition.X)+car_width-1],
					gameData.Car[line*car_width:line*car_width+car_width-1])
			}

			// Applying the score
			scoreStr := fmt.Sprintf("%d", roundData.player.Score)
			for i := range scoreStr {
				data[i] = byte(scoreStr[i])

			}

			_, err = conn.Write(data)
			if err != nil {
				return
			}
			time.Sleep(time.Duration(roundData.Speed) * time.Millisecond)
		}
	}
}

func main() {
	var logFile string
	conf := &Config{}

	flag.StringVar(&logFile, "l", "/var/log/race.log", "Log file")
	flag.StringVar(&conf.AcidPath, "a", "/Users/leoleovich/go/src/github.com/leoleovich/race/artifacts", "Artifacts location")
	flag.StringVar(&conf.ScorePath, "s", "/Users/leoleovich/go/src/github.com/leoleovich/race/artifacts", "Score location")
	flag.Parse()

	logfile, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	conf.Log = log.New(logfile, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)

	l, err := net.Listen("tcp", ":4242")
	if err != nil {
		conf.Log.Println(err)
		os.Exit(2)
	}
	defer l.Close()

	gameData := GameData{}
	gameData.Roads = [][]byte{generateRoad(false), generateRoad(true)}
	gameData.Car, _ = getAcid(conf, "car.txt")
	gameData.Clear, _ = getAcid(conf, "clear.txt")
	gameData.Splash, _ = getAcid(conf, "splash.txt")
	scoreData, _ := getAcid(conf, "score.json")
	err = json.Unmarshal(scoreData, &gameData.Top)

	for {
		conn, err := l.Accept()
		if err != nil {
			conf.Log.Println("Failed to accept request", err)
		}
		go round(conf, conn, &gameData)
	}
}

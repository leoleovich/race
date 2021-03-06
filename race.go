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
	"sort"
	"strings"
	"time"
)

const road_width = 41
const road_lenght = 20
const car_width = 14
const car_lenght = 7
const result_width = 77
const max_players_in_top = 10
const speed_factor = 10500

type Config struct {
	Log                 *log.Logger
	AcidPath, ScorePath string
}

type Point struct {
	X, Y int
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

type Player struct {
	Name  string
	Score int64
}

type Players []Player

func (p Players) Len() int           { return len(p) }
func (p Players) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p Players) Less(i, j int) bool { return p[i].Score > p[j].Score }

func generateRoads(size int) [][]byte {
	roads := make([][]byte, size)
	for offset := 0; offset < size; offset++ {
		roads[offset] = make([]byte, road_width*road_lenght)
		for row := 0; row < road_lenght; row++ {
			for column := 0; column < road_width; column++ {
				var symbol byte
				if column == 0 || column == road_width-3 {
					symbol = byte('|')
				} else if column == road_width-2 {
					symbol = byte('\r')
				} else if column == road_width-1 {
					symbol = byte('\n')
				} else if column == road_width/2-1 {
					if (row-offset)%size == 0 {
						symbol = byte('|')
					} else {
						symbol = byte(' ')
					}
				} else {
					symbol = byte(' ')
				}
				roads[offset][row*road_width+column] = symbol
			}
		}
	}
	return roads
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

func updatePosition(conf *Config, conn net.Conn, roundData *RoundData, gameData *GameData) {
	for {
		direction := make([]byte, 1)

		// Read all possible bytes and try to find a sequence of:
		// ESC [ cursor_key
		escpos := 0
		for {
			_, err := conn.Read(direction)
			if err != nil {
				return
			}

			// Check if telnet want to negotiate something
			if escpos == 0 && direction[0] == 255 {
				readTelnet(conn)
			} else if escpos == 0 && direction[0] == 27 {
				escpos = 1
			} else if direction[0] == 3 {
				gameOver(conf, conn, roundData, gameData)
				conn.Close()
				return
			} else if escpos == 1 && direction[0] == 91 {
				escpos = 2
			} else if escpos == 2 {
				break
			}
		}

		switch direction[0] {
		case 68:
			// Left
			roundData.CarPosition.X--
		case 67:
			// Right
			roundData.CarPosition.X++
		case 65:
			// Up
			roundData.CarPosition.Y--
		case 66:
			// Down
			roundData.CarPosition.Y++
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
	conn.Write(gameData.Clear)
	conn.Write(gameData.Splash)
	io := bufio.NewReader(conn)
	line, err := io.ReadString('\n')
	if err != nil {
		conf.Log.Println("Error while name reading", err)
		return "", err
	}
	name := strings.Replace(strings.Replace(line, "\n", "", -1), "\r", "", -1)
	if name == "" {
		conf.Log.Println("Empty name")
		return "", errors.New("Empty name")
	}
	if len(name) > result_width/2 {
		conf.Log.Println("Too long name")
		return "", errors.New("Too long name")
	}
	return name, nil
}

func recalculatePlayers(gameData *GameData, roundData *RoundData) {
	// Then we check on which place is current player
	processed := false
	for _, player := range gameData.Top {
		if roundData.player.Score >= player.Score || len(gameData.Top) < max_players_in_top {
			// Squash all players
			for i, player := range gameData.Top {
				if roundData.player.Name == player.Name {
					if roundData.player.Score >= player.Score {
						gameData.Top[i].Score = roundData.player.Score
					}
					processed = true
					break
				}
			}
			break
		}
	}

	// Insert new record to the end of slice
	if !processed {
		gameData.Top = append(gameData.Top, roundData.player)
	}

	// Resort the slice
	sort.Sort(Players(gameData.Top))

	// Remove slowest user if top is full
	if len(gameData.Top) >= max_players_in_top {
		gameData.Top = gameData.Top[:max_players_in_top]
	}
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
		roundData.GameOver[result_width-2-len(roundData.player.Name)+i] = byte(char)
	}

	recalculatePlayers(gameData, roundData)

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
			roundData.GameOver[(2+place)*result_width+result_width-2-len(player.Name)+i] = byte(char)
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
	if roundData.player.Score >= 2000 {
		roundData.Speed = 60
		roundData.BonusFactor = 1
		roundData.BombFactor = 20
	} else if roundData.player.Score >= 1500 {
		roundData.Speed = 70
		roundData.BombFactor = 10
	} else if roundData.player.Score >= 1000 {
		roundData.Speed = 80
		roundData.BombFactor = 5
	} else if roundData.player.Score >= 600 {
		roundData.BonusFactor = 100
		roundData.BombFactor = 2
		roundData.Speed = 90
	} else if roundData.player.Score >= 200 {
		roundData.BonusFactor = 10
		roundData.BombFactor = 5
		roundData.Speed = 100
	} else if roundData.player.Score >= 50 {
		roundData.Speed = 150
	}

}

func checkPosition(conf *Config, conn net.Conn, roundData *RoundData, gameData *GameData) bool {
	if roundData.CarPosition.X < 1 || roundData.CarPosition.X > road_width-1-car_width-1 || roundData.CarPosition.Y < 1 || roundData.CarPosition.Y > road_lenght-car_lenght-1 {
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

func initTelnet(conn net.Conn) error {
	// https://tools.ietf.org/html/rfc854
	telnetOptions := []byte{
		255, 253, 34, // IAC DO LINEMODE
		255, 250, 34, 1, 0, 255, 240, // IAC SB LINEMODE MODE 0 IAC SE
		255, 251, 1, // IAC WILL ECHO
	}
	_, err := conn.Write(telnetOptions)
	if err != nil {
		return err
	}
	return nil
}

func readTelnet(conn net.Conn) error {
	// https://tools.ietf.org/html/rfc854
	reply := make([]byte, 1)
	bytesRead := 0
	shortCommand := false

	for {
		_, err := conn.Read(reply)
		if err != nil {
			return err
		}
		bytesRead++

		if reply[0] != 250 && bytesRead == 1 {
			shortCommand = true
		}

		if shortCommand && bytesRead == 2 {
			return nil
		} else if reply[0] == 240 {
			return nil
		}
	}
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

	if initTelnet(conn) != nil {
		return
	}

	go updateScore(&roundData)
	go updatePosition(conf, conn, &roundData, gameData)

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
				roundData.bonusPosition.X, roundData.bonusPosition.Y = rand.Intn(road_width-4)+1, 0
			}

			// Applying the bomb
			if roundData.bombPosition.Y < road_lenght {
				data[roundData.bombPosition.Y*road_width+roundData.bombPosition.X] = byte('X')
				roundData.bombPosition.Y++
			} else if rand.Int()%roundData.BombFactor == 0 {
				roundData.bombPosition.X, roundData.bombPosition.Y = rand.Intn(road_width-4)+1, 0
			}

			// Applying the Car
			for line := 0; line < car_lenght; line++ {
				copy(data[((roundData.CarPosition.Y+line)*road_width+roundData.CarPosition.X):((roundData.CarPosition.Y+line)*road_width+roundData.CarPosition.X)+car_width-1],
					gameData.Car[line*car_width:line*car_width+car_width-1])
			}

			// Applying the score
			scoreStr := fmt.Sprintf("Score: %d", roundData.player.Score)
			for i := range scoreStr {
				data[(road_lenght-1)*road_width+i] = byte(scoreStr[i])
			}
			// Applying the speed
			speedStr := fmt.Sprintf("Speed: %d km/h", speed_factor/roundData.Speed)
			for i := range speedStr {
				data[road_lenght*road_width-len(speedStr)-2+i] = byte(speedStr[i])
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
	var port int
	conf := &Config{}

	flag.StringVar(&logFile, "l", "/var/log/race.log", "Log file")
	flag.IntVar(&port, "p", 4242, "Port to listen")
	flag.StringVar(&conf.AcidPath, "a", "/Users/leoleovich/go/src/github.com/leoleovich/race/artifacts", "Artifacts location")
	flag.StringVar(&conf.ScorePath, "s", "/Users/leoleovich/go/src/github.com/leoleovich/race/artifacts", "Score location")
	flag.Parse()

	logfile, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	conf.Log = log.New(logfile, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		conf.Log.Println(err)
		os.Exit(2)
	}
	defer l.Close()

	gameData := GameData{}
	gameData.Roads = generateRoads(2)
	gameData.Car, _ = getAcid(conf, "car.txt")
	gameData.Clear = []byte{27, 91, 50, 74, 27, 91, 72}
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

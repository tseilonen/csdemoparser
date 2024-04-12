package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync" // Import sync package for mutex
	"time"

	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

func main() {
	defer TimeTrack(time.Now())

	// slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.SetLogLoggerLevel(slog.LevelInfo)
	// slog.SetLogLoggerLevel(slog.LevelError)

	demosDir := "data/demos/"
	parsedDir := "data/parsed/"

	// Ensure the parsed directory exists, create it if it doesn't
	if _, err := os.Stat(parsedDir); os.IsNotExist(err) {
		err := os.MkdirAll(parsedDir, 0755)
		if err != nil {
			slog.Error(fmt.Sprintf("Error creating parsed directory: %s", err))
		}
	}

	// Get list of files in the parsed directory
	parsedFiles, err := os.ReadDir(parsedDir)
	if err != nil {
		slog.Error(fmt.Sprintf("Error reading parsed directory: %s", err))
	}

	// Create a map to store the names of parsed files
	parsedMap := make(map[string]bool)
	for _, file := range parsedFiles {
		// Trim the .json extension and store the filename
		name := strings.TrimSuffix(file.Name(), ".dem_scoreboard.json")
		parsedMap[name] = true
	}

	// Read the demos directory
	demos, err := os.ReadDir(demosDir)
	if err != nil {
		slog.Error(fmt.Sprintf("Error reading demos directory: %s", err))
	}

	// Loop through the demos directory, and parse demos
	for _, demo := range demos {
		if strings.HasSuffix(demo.Name(), ".dem") {
			filename := strings.TrimSuffix(demo.Name(), ".dem")
			// Check if the demo hasn't been parsed already

			if _, exists := parsedMap[filename]; !exists {
				if true { //!strings.Contains(filename, "2024-01") && !strings.Contains(filename, "_-1") {
					err := parseSingleDemo(demosDir, demo.Name(), parsedDir)

					if err != nil {
						slog.Error(fmt.Sprintf("%v parsing failed", demo.Name()))
						slog.Error(fmt.Sprint(err))
					} else {
						slog.Info(fmt.Sprintf("%v parsing succeeded", demo.Name()))
					}
				}
			}
			// break // ---------------------------------------------PARSE ONLY ONE DEMO FOR DEBUGGINGS----------------------------------------------------------------- //
		}
	}
}

func parseSingleDemo(demosDir string, filename string, parsedDir string) (err error) {
	defer TimeTrackFile(time.Now(), filename)

	slog.Info(fmt.Sprintf("%v started parsing", filename))

	file, err := os.Open(demosDir + filename)
	if err != nil {
		slog.Error(fmt.Sprintf("Error opening demo file: %v", filename))
		return err
	}
	defer file.Close()

	// Parse the demo file
	p := dem.NewParser(file)
	defer p.Close()

	var scoreboard Scoreboard
	var kniferound []KniferoundStats
	var scoreboardMutex sync.Mutex // Mutex to synchronize access to scoreboard
	var roundStats RoundStats
	var matchStarted bool
	var scoreboardInitialized bool

	previousFlashId := 0 // For some reason flashexplode events appear twice, so with these we can keep track of counted flashes
	var previousFlashThrower *common.Player

	// Register event handlers
	p.RegisterEventHandler(func(e events.MatchStart) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		updateKnife := false

		if !reflect.DeepEqual(scoreboard, Scoreboard{}) && len(kniferound) == 0 && scoreboard.RoundsPlayed == 1 && scoreboard.knifeRoundMatch {
			for _, ps := range scoreboard.PlayerScores {
				kniferound = append(kniferound, KniferoundStats{SteamID: ps.SteamID, Kills: ps.Kills, Assists: ps.Assists, Deaths: ps.Deaths})
			}

			updateKnife = true
		}

		// Initialize the scoreboard at the beginning of the match
		scoreboard = initializeScoreboard(p.GameState())

		// string to int
		i, err := strconv.Atoi(p.GameState().Rules().ConVars()["mp_maxrounds"])
		if err != nil {
			slog.Error("mp_maxrounds is not a number!")
			panic(err)
		}

		scoreboard.MaxRounds = i

		if kniferound != nil && updateKnife {
			for _, pk := range kniferound {
				for i, ps := range scoreboard.PlayerScores {
					if pk.SteamID == ps.SteamID {
						scoreboard.PlayerScores[i].KnifeRoundKills = pk.Kills
						scoreboard.PlayerScores[i].KnifeRoundAssists = pk.Assists
						scoreboard.PlayerScores[i].KnifeRoundDeaths = pk.Deaths
						break
					}
				}
			}
		}

		if !matchStarted && scoreboardInitialized {
			slog.Warn("Scoreboard was initialized before match start. Stats might have something funky going on.")
		}

		matchStarted = true
		scoreboardInitialized = true

		slog.Debug("Match start")
		slog.Debug(fmt.Sprint(scoreboard))
	})

	p.RegisterEventHandler(func(e events.RoundStart) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		if !matchStarted && !scoreboardInitialized {
			slog.Warn("Demofile doesn't have match start event in the beginning of file. Something will likely fail. Initializing scoreboard.")
			scoreboard = initializeScoreboard(p.GameState())
			scoreboardInitialized = true
		}

		cts := p.GameState().TeamCounterTerrorists()
		ts := p.GameState().TeamTerrorists()

		roundStats = initializeRoundStats(scoreboard, cts, ts)

		slog.Debug(fmt.Sprintf("Round %v start", scoreboard.RoundsPlayed+1))
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Update the scoreboard at the end of each round

		// if !mat

		var ps *PlayerScore
		for _, player := range p.GameState().Participants().Playing() {
			scoreboard.PlayerScores, ps = scoreboard.getAddPlayerScore(player)
			ps.Kills = player.Kills()
			ps.Assists = player.Assists()
			ps.Deaths = player.Deaths()
			ps.Mvps = player.MVPs()
			ps.MoneySpentTotal = player.MoneySpentTotal()
			ps.TeamRounds = player.TeamState.Score()

			slog.Debug(fmt.Sprintf("Player %v	Team id %v", player.Name, ps.playerRef.TeamState.ID()))

			switch roundStats.KillsOnRound[ps.SteamID] {
			case 2:
				ps.Enemy2k += 1
			case 3:
				ps.Enemy3k += 1
			case 4:
				ps.Enemy4k += 1
			case 5:
				ps.Enemy5k += 1
			}

			if roundStats.ClutchingPlayer != nil && roundStats.ClutchingPlayer.Team == e.Winner && roundStats.ClutchingPlayer.SteamID64 == player.SteamID64 {
				switch roundStats.EnemiesToClutch {
				case 1:
					ps.ClutchV1Wins += 1
				case 2:
					ps.ClutchV2Wins += 1
				case 3:
					ps.ClutchV3Wins += 1
				case 4:
					ps.ClutchV4Wins += 1
				case 5:
					ps.ClutchV5Wins += 1
				}
			}

			if roundStats.Clutch1V1 != nil && roundStats.Clutch1V1.Team == e.Winner && roundStats.Clutch1V1.SteamID64 == player.SteamID64 {
				ps.ClutchV1Wins += 1
			}

		}

		scoreboard.RoundsPlayed = p.GameState().TotalRoundsPlayed()

		scoreboard.addResidualDamage(roundStats.RoundHealths)

		roundStats.RoundEnded = true

		slog.Debug(fmt.Sprintf("Round %v ended", scoreboard.RoundsPlayed))
		slog.Debug(fmt.Sprint(scoreboard))

	})

	p.RegisterEventHandler(func(e events.Kill) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Ensure scoreboard is initialized
		if scoreboard.PlayerScores == nil {
			return
		}

		slog.Debug(fmt.Sprintf("%v killed %v with %v", e.Killer, e.Victim, e.Weapon))

		killer := scoreboard.getPlayerScore(e.Killer)
		victim := scoreboard.getPlayerScore(e.Victim)
		var victimTeamAlive int

		// Clutchi laskentaa
		victimTeam := getPlayerTeam(e.Victim)
		if victimTeam == 2 {
			roundStats.TAlive -= 1
			victimTeamAlive = roundStats.TAlive
		}
		if victimTeam == 3 {
			roundStats.CTAlive -= 1
			victimTeamAlive = roundStats.CTAlive
		}

		if victimTeamAlive == 1 && !roundStats.RoundEnded {
			if roundStats.ClutchingPlayer == nil {
				for _, p := range e.Victim.TeamState.Opponent.Members() {
					if p.IsAlive() {
						roundStats.EnemiesToClutch += 1
					}
				}
			}

			members := e.Victim.TeamState.Members()
			for i, p := range members {
				if p.IsAlive() && p.SteamID64 != e.Victim.SteamID64 {
					ps := scoreboard.getPlayerScore(members[i])

					if roundStats.ClutchingPlayer != nil {
						roundStats.Clutch1V1 = members[i]
						ps.ClutchV1Count += 1
					} else {
						roundStats.ClutchingPlayer = members[i]

						switch roundStats.EnemiesToClutch {
						case 1:
							ps.ClutchV1Count += 1
						case 2:
							ps.ClutchV2Count += 1
						case 3:
							ps.ClutchV3Count += 1
						case 4:
							ps.ClutchV4Count += 1
						case 5:
							ps.ClutchV5Count += 1
						}
					}

					break
				}
			}
		}

		if e.Weapon != nil {
			killer.KillsByWeapon[e.Weapon.String()] += 1
			victim.DeathsByWeapon[e.Weapon.String()] += 1
		}

		/*
			0	teamkill
			1	smoke
			2	wallbang
			3	headshot
			4	no scope
			5	blind
			6	flash
			7	suicide
		*/

		killtype := uint32(
			boolToInt(getPlayerTeam(e.Killer) == getPlayerTeam(e.Victim) && killer.SteamID != victim.SteamID)*1 +
				boolToInt(e.ThroughSmoke)*2 +
				boolToInt(e.PenetratedObjects > 0)*4 +
				boolToInt(e.IsHeadshot)*8 +
				boolToInt(e.NoScope)*16 +
				boolToInt(e.AttackerBlind)*32 +
				boolToInt(e.AssistedFlash)*64 +
				boolToInt(killer.SteamID == victim.SteamID)*128)

		killer.KillsByType[killtype] += 1
		victim.DeathsByType[killtype] += 1

		if e.Weapon.Type == 407 { // 407 World damage
			victim.Suicides += 1
		} else {
			if getPlayerTeam(e.Killer) != getPlayerTeam(e.Victim) {
				roundStats.KillsOnRound[killer.SteamID] += 1

				if e.PenetratedObjects > 0 {
					killer.WallBangKills += 1
					victim.WallBangDeaths += 1
				}
				if e.IsHeadshot {
					killer.HeadshotKills += 1
					victim.HeadshotDeaths += 1
				}
				if e.AttackerBlind {
					killer.BlindKills += 1
					victim.BlindDeaths += 1
				}
				if e.NoScope {
					killer.NoscopeKills += 1
					victim.NoscopeDeaths += 1
				}
				if e.ThroughSmoke {
					killer.SmokeKills += 1
					victim.SmokeDeaths += 1
				}
				if !roundStats.EnemiesKilled {
					roundStats.EnemiesKilled = true
					killer.EntryCount += 1
					killer.EntryWins += 1
					victim.EntryCount += 1
				}

				if e.AssistedFlash {
					killer.FlashKills += 1
					victim.FlashDeaths += 1

					assister := scoreboard.getPlayerScore(e.Assister)
					assister.FlashAssists += 1
				}
			} else {
				if e.PenetratedObjects > 0 {
					killer.TeamWallBangKills += 1
					victim.TeamWallBangDeaths += 1
				}
				if e.IsHeadshot {
					killer.TeamHeadshotKills += 1
					victim.TeamHeadshotDeaths += 1
				}
				if e.AttackerBlind {
					killer.TeamBlindKills += 1
					victim.TeamBlindDeaths += 1
				}
				if e.NoScope {
					killer.TeamNoscopeKills += 1
					victim.TeamNoscopeDeaths += 1
				}
				if e.ThroughSmoke {
					killer.TeamSmokeKills += 1
					victim.TeamSmokeDeaths += 1
				}

				if e.AssistedFlash {
					killer.TeamFlashKills += 1
					victim.TeamFlashDeaths += 1

					assister := scoreboard.getPlayerScore(e.Assister)
					assister.TeamFlashAssists += 1
				}
			}
		}
	})

	p.RegisterEventHandler(func(e events.OtherDeath) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Ensure scoreboard is initialized
		if scoreboard.PlayerScores == nil {
			return
		}

		killer := scoreboard.getPlayerScore(e.Killer)

		if e.OtherType == "chicken" {
			killer.ChickenKills += 1
		}

	})

	p.RegisterEventHandler(func(e events.GrenadeEventIf) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Ensure scoreboard is initialized
		if scoreboard.PlayerScores == nil {
			return
		}

		thrower := scoreboard.getPlayerScore(e.Base().Thrower)

		slog.Debug(fmt.Sprintf("%v throwed %v", e.Base().Thrower, e.Base().Grenade))

		switch e.(type) {
		case events.FlashExplode:
			if previousFlashThrower != e.Base().Thrower || previousFlashId != e.Base().GrenadeEntityID {
				thrower.FlashesThrown += 1
			}
			previousFlashId = e.Base().GrenadeEntityID
			previousFlashThrower = e.Base().Thrower
		case events.HeExplode:
			thrower.HesThrown += 1
		case events.SmokeStart:
			thrower.SmokesThrown += 1
		case events.DecoyStart:
			thrower.DecoysThrown += 1
		}

	})

	p.RegisterEventHandler(func(e events.InfernoStart) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Ensure scoreboard is initialized
		if scoreboard.PlayerScores == nil {
			return
		}

		slog.Debug(fmt.Sprintf("%v throwed a fire grenade", e.Inferno.Thrower()))

		thrower := scoreboard.getPlayerScore(e.Inferno.Thrower())
		thrower.BurnsThrown += 1

	})

	p.RegisterEventHandler(func(e events.WeaponFire) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Ensure scoreboard is initialized
		if scoreboard.PlayerScores == nil {
			return
		}

		shooter := scoreboard.getPlayerScore(e.Shooter)
		shooter.ShotsFired += 1

	})

	p.RegisterEventHandler(func(e events.WeaponReload) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Ensure scoreboard is initialized
		if scoreboard.PlayerScores == nil {
			return
		}

		shooter := scoreboard.getPlayerScore(e.Player)
		shooter.Reloads += 1

	})

	p.RegisterEventHandler(func(e events.PlayerFlashed) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Ensure scoreboard is initialized
		if scoreboard.PlayerScores == nil {
			return
		}

		slog.Debug(fmt.Sprintf("%v flashed %v for %.2f seconds", e.Attacker, e.Player, e.Player.FlashDuration))

		attacker := scoreboard.getPlayerScore(e.Attacker)
		receiver := scoreboard.getPlayerScore(e.Player)

		if e.Player != nil {
			if getPlayerTeam(e.Player) != getPlayerTeam(e.Attacker) {
				if e.Player.FlashDuration > 1.1 {
					attacker.EnemiesFullFlashed += 1
					receiver.FullFlashesReceived += 1
				} else {
					attacker.EnemiesHalfFlashed += 1
					receiver.HalfFlashesReceived += 1
				}
			} else {
				if attacker != receiver {
					if e.Player.FlashDuration > 1.1 {
						receiver.TeamFullFlashesReceived += 1
						attacker.TeammatesFullFlashed += 1
					} else {
						receiver.TeamHalfFlashesReceived += 1
						attacker.TeammatesHalfFlashed += 1
					}
				} else {
					if e.Player.FlashDuration > 1.1 {
						attacker.SelfFullFlashes += 1
					} else {
						attacker.SelfHalfFlashes += 1
					}
				}
			}
		} else {
			if getPlayerTeam(e.Player) != getPlayerTeam(e.Attacker) {
				attacker.EnemiesFullFlashed += 1
			} else {
				attacker.TeammatesFullFlashed += 1
			}
		}

		// if getPlayerTeam(e.Player) != getPlayerTeam(e.Attacker) {
		// 	attacker.EnemiesFlashed += 1
		// 	receiver.FlashesReceived += 1
		// } else {
		// 	if attacker != receiver {
		// 		receiver.TeamFlashesReceived += 1
		// 		attacker.TeammatesFlashed += 1
		// 	} else {
		// 		attacker.SelfFlashes += 1
		// 	}
		// }
	})

	p.RegisterEventHandler(func(e events.PlayerHurt) {
		scoreboardMutex.Lock() // Lock the mutex before accessing scoreboard
		defer scoreboardMutex.Unlock()

		// Ensure scoreboard is initialized
		if scoreboard.PlayerScores == nil {
			return
		}

		// Update the damage done by the player
		attacker := scoreboard.getPlayerScore(e.Attacker)
		receiver := scoreboard.getPlayerScore(e.Player)

		slog.Debug(fmt.Sprintf("%v caused %v damage to %v with %v", e.Attacker, e.HealthDamageTaken, e.Player, e.Weapon))

		if scoreboard.knifeRoundMatch && e.Weapon.Type != common.EqKnife && e.HealthDamageTaken > 0 {
			scoreboard.knifeRoundMatch = false
			slog.Debug("scoreboard.KnifeRoundMatch set to false")
		}

		if attacker != nil {
			//There's a bug/feature in the demoinfocs package that requires this complicated damage calculation in some edge cases
			dmg := 0

			if e.Health == 0 && e.HealthDamageTaken > e.HealthDamage {
				roundStats.RoundHealths.updateDamager(e.Player, e.Attacker)
			} else {
				roundStats.RoundHealths.updateMinHealth(e.Player, e.Health)
				dmg = e.HealthDamageTaken
			}

			if getPlayerTeam(e.Player) != getPlayerTeam(e.Attacker) {
				switch e.Weapon.Type {
				case 502: // Molotov
					attacker.BurnDamageDealt += dmg
				case 503: // Incendiary
					attacker.BurnDamageDealt += dmg
				case 506: // He
					attacker.HeDamageDealt += dmg
				}

				attacker.DamageDone += dmg
				attacker.ShotsOnEnemies += 1

				switch e.Weapon.Type {
				case 502: // Molotov
					receiver.BurnDamageReceived += dmg
				case 503: // Incendiary
					receiver.BurnDamageReceived += dmg
				case 506: // He
					receiver.HeDamageReceived += dmg
				}

				receiver.DamageReceived += dmg

			} else {
				switch e.Weapon.Type {
				case 502: // Molotov
					attacker.TeamBurnDamageDealt += dmg
				case 503: // Incendiary
					attacker.TeamBurnDamageDealt += dmg
				case 506: // He
					attacker.TeamHeDamageDealt += dmg
				}

				attacker.TeamDamageDone += dmg
				attacker.ShotsOnTeammates += 1

				switch e.Weapon.Type {
				case 502: // Molotov
					receiver.TeamBurnDamageReceived += dmg
				case 503: // Incendiary
					receiver.TeamBurnDamageReceived += dmg
				case 506: // He
					receiver.TeamHeDamageReceived += dmg
				}

				if receiver == attacker {
					switch e.Weapon.Type {
					case 502: // Molotov
						receiver.BurnSelfDamage += dmg
					case 503: // Incendiary
						receiver.BurnSelfDamage += dmg
					case 506: // He
						receiver.HeSelfDamage += dmg
					}
				}

				receiver.TeamDamageReceived += dmg

			}
		}
	})

	// Parse the demo
	err = p.ParseToEnd()
	if err != nil {
		if errors.Is(err, dem.ErrUnexpectedEndOfDemo) {
			slog.Warn(fmt.Sprintf("%v file incomplete. File has only %v complete rounds. Writing json still.", filename, scoreboard.RoundsPlayed))
		} else {
			slog.Error(fmt.Sprintf("Error parsing demo: %v", filename))
			return err
		}
	}

	scoreboard.MapName = p.Header().MapName

	scoreboard.updatePostMatchStats()

	err = scoreboard.saveJson(filename+"_scoreboard.json", parsedDir)
	if err != nil {
		slog.Error(fmt.Sprintf("Error saving scoreboard to CSV from demo: %v", filename))
		return err
	}

	return
}

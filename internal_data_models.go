package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"time"

	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

func (p *PlayerScore) calculateADR(roundsPlayed int) {
	// Calculate ADR (Average Damage per Round)
	if roundsPlayed == 0 {
		p.ADR = 0.0
	}

	p.ADR = float64(p.DamageDone) / float64(roundsPlayed)
}

func (p *PlayerScore) calculateKAST(roundsPlayed int) {
	// Calculate ADR (Average Damage per Round)
	if roundsPlayed == 0 {
		p.Kast = 0.0
	}

	p.Kast = p.Kast / float64(roundsPlayed) * 100
}

func (rhs RoundHealths) updateDamager(p *common.Player, d *common.Player) {
	if p == nil || d == nil {
		return
	}

	for i, rh := range rhs {
		if rh.SteamID == p.SteamID64 {
			rhs[i].PlayerWhoGetsTheDamage = d.SteamID64
			break
		}
	}
}

func (rhs RoundHealths) updateMinHealth(p *common.Player, health int) {
	if p == nil {
		return
	}

	for i, rh := range rhs {
		if rh.SteamID == p.SteamID64 {
			if rh.MinHealthAboveZero > health && health > 0 {
				rhs[i].MinHealthAboveZero = health
			}
			break
		}
	}
}

func initializeRoundStats(sb Scoreboard, ctTS *common.TeamState, tTS *common.TeamState) RoundStats {
	var rs RoundStats

	rs.RoundHealths = initializeRoundHealths(sb.PlayerScores)
	rs.KillsOnRound = make(map[uint64]int)
	rs.Kast = make(map[uint64]bool)
	rs.TimeOfDeath = make(map[uint64]time.Duration)

	for _, p := range ctTS.Members() {
		if p.IsAlive() {
			rs.CTAlive += 1
		}
	}

	for _, p := range tTS.Members() {
		if p.IsAlive() {
			rs.TAlive += 1
		}
	}

	rs.EnemiesKilled = false
	rs.RoundEnded = false
	return rs
}

func initializeRoundHealths(scores []PlayerScore) RoundHealths {
	var rhs RoundHealths
	for _, ps := range scores {
		rhs = append(rhs, RoundHealth{
			SteamID:            ps.SteamID,
			MinHealthAboveZero: 100,
		})
	}
	return rhs
}

func initializeScoreboard(gs dem.GameState) Scoreboard {
	sb := Scoreboard{}

	sb.knifeRoundMatch = true

	sb.TeamMemebers = make(map[int][]uint64)

	cts := gs.TeamCounterTerrorists()
	sb.TeamMemebers[cts.ID()] = []uint64{}

	ts := gs.TeamTerrorists()
	sb.TeamMemebers[ts.ID()] = []uint64{}

	if len(sb.TeamMemebers[cts.ID()]) > 5 {
		slog.Warn(fmt.Sprintf("Team %v (%v) player count %v. Count exceeded in initializeScoreboard.", cts.ID(), cts.ClanName(), len(sb.TeamMemebers[cts.ID()])))
	}

	if len(sb.TeamMemebers[ts.ID()]) > 5 {
		slog.Warn(fmt.Sprintf("Team %v (%v) player count %v. Count exceeded in initializeScoreboard.", ts.ID(), ts.ClanName(), len(sb.TeamMemebers[ts.ID()])))
	}

	for _, player := range gs.Participants().Playing() {
		sb.PlayerScores, _ = sb.getAddPlayerScore(player)
	}

	sb.KDTypeBits = map[int]string{0: "teamkill", 1: "through smoke", 2: "wallbang", 3: "headshot", 4: "no scope", 5: "attacker blind", 6: "victim flashed", 7: "suicide"}

	return sb
}

func (sb *Scoreboard) addResidualDamage(rhs RoundHealths) {
	for _, rh := range rhs {
		if rh.PlayerWhoGetsTheDamage != 0 {
			for i, ps := range sb.PlayerScores {
				if ps.SteamID == rh.PlayerWhoGetsTheDamage {
					sb.PlayerScores[i].DamageDone += rh.MinHealthAboveZero
				}
			}
		}
	}
}

func (sb *Scoreboard) updatePostMatchStats() {
	// Calculate winner
	// Determine the team with the most rounds
	// Determine the number of zeroround players
	maxRounds := 0
	var winnerTeamID int
	var zeroRoundPlayers []int
	for i, player := range sb.PlayerScores {
		if player.TeamRounds > maxRounds {
			maxRounds = player.TeamRounds
			winnerTeamID = player.TeamId
		}

		if player.PlayedRounds == 0 {
			zeroRoundPlayers = append(zeroRoundPlayers, i)
		}
	}

	sb.WinnerTeamID = winnerTeamID
	sb.WinnerTeam = sb.TeamNames[winnerTeamID]

	// Remove zeroroundplayers
	extra := 0
	lastValid := 0
	for i, rm := range zeroRoundPlayers {
		valueToCheck := len(sb.PlayerScores) - (i + 1 + extra)
		for slices.Contains(zeroRoundPlayers, valueToCheck) {
			extra += 1
			valueToCheck = len(sb.PlayerScores) - (i + 1 + extra)
		}

		lastValid = max(valueToCheck, 0)
		sb.PlayerScores[rm] = sb.PlayerScores[lastValid]
	}

	if len(zeroRoundPlayers) > 0 {
		slog.Warn(fmt.Sprintf("Removing %v zeroround players", len(zeroRoundPlayers)))
		sb.PlayerScores = sb.PlayerScores[:len(sb.PlayerScores)-len(zeroRoundPlayers)]
	}

	// Write scoreboard in order of teams and kills
	sort.Slice(sb.PlayerScores, func(i, j int) bool {
		if sb.PlayerScores[i].TeamId != sb.PlayerScores[j].TeamId {
			return sb.PlayerScores[i].TeamId > sb.PlayerScores[j].TeamId
		}
		return sb.PlayerScores[i].Kills > sb.PlayerScores[j].Kills
	})

	// Calculate ADR and KAST for each player
	for i := range sb.PlayerScores {
		sb.PlayerScores[i].calculateADR(sb.RoundsPlayed)
		sb.PlayerScores[i].calculateKAST(sb.RoundsPlayed)
	}

}

func (sb *Scoreboard) getPlayerScore(p *common.Player) *PlayerScore {
	if p != nil && p.Name != "SourceTV" {
		// if p != nil {
		_, ps := sb.getAddPlayerScore(p)
		return ps
	}

	// Return empty dummy so PlayerScore doesn't need to be checked for nil always
	dummy := &PlayerScore{}
	dummy.KillsByWeapon = make(map[string]int)
	dummy.DeathsByWeapon = make(map[string]int)
	dummy.KillsByType = make(map[uint32]int)
	dummy.DeathsByType = make(map[uint32]int)

	return dummy
}

func (sb *Scoreboard) getAddPlayerScore(p *common.Player) ([]PlayerScore, *PlayerScore) {
	if id := getSteamID64(p); id > 0 {
		for i, ps := range sb.PlayerScores {
			if ps.SteamID == id {
				return sb.PlayerScores, &sb.PlayerScores[i]
			}
		}
	}

	// If sides have changed and a player is added, we have to change team ids, because TeamState.ID() isn't a constant even though it is supposed to be
	if sb.MaxRounds/2 < sb.RoundsPlayed && !sb.teamsSwapped {
		sb.TeamMemebers = make(map[int][]uint64)

		for i, ps := range sb.PlayerScores {
			newTeam := ps.playerRef.TeamState.ID()
			sb.PlayerScores[i].TeamId = newTeam
			sb.TeamMemebers[newTeam] = append(sb.TeamMemebers[newTeam], ps.SteamID)
		}

		sb.teamsSwapped = true
	}

	sb.TeamMemebers[p.TeamState.ID()] = append(sb.TeamMemebers[p.TeamState.ID()], p.SteamID64)

	ClanName := ""

	if p.TeamState != nil {
		ClanName = p.TeamState.ClanName()
	}

	sb.PlayerScores = append(sb.PlayerScores, PlayerScore{
		SteamID:   p.SteamID64,
		Nickname:  p.Name,
		Team:      ClanName,
		TeamId:    p.TeamState.ID(),
		playerRef: p,
	})

	sb.PlayerScores[len(sb.PlayerScores)-1].KillsByWeapon = make(map[string]int)
	sb.PlayerScores[len(sb.PlayerScores)-1].DeathsByWeapon = make(map[string]int)
	sb.PlayerScores[len(sb.PlayerScores)-1].KillsByType = make(map[uint32]int)
	sb.PlayerScores[len(sb.PlayerScores)-1].DeathsByType = make(map[uint32]int)

	if len(sb.TeamMemebers[p.TeamState.ID()]) > 5 {
		slog.Warn(fmt.Sprintf("Team %v (%v) player count %v. Added %v. Teammembers %v", p.TeamState.ID(), ClanName, len(sb.TeamMemebers[p.TeamState.ID()]), p.Name, sb.TeamMemebers[p.TeamState.ID()]))
	}

	return sb.PlayerScores, &sb.PlayerScores[len(sb.PlayerScores)-1]
}

func (sb *Scoreboard) saveJson(filename string, parsedDir string) error {
	file, err := os.Create(parsedDir + filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.Encode(sb)

	return nil
}

type RoundStats struct {
	KillsOnRound    map[uint64]int
	CTAlive         int
	TAlive          int
	EnemiesKilled   bool
	RoundHealths    RoundHealths
	ClutchingPlayer *common.Player
	Clutch1V1       *common.Player
	EnemiesToClutch int
	RoundEnded      bool       // Events after round end don't count towards clutches so, we need to track the round status
	KillTimes       []KillTime // Kill times need to be tracked for trade calculation. Map key is killer id
	Kast            map[uint64]bool
	TimeOfDeath     map[uint64]time.Duration
}

type KillTime struct {
	Timestamp time.Duration
	VictimID  uint64
	KillerID  uint64
}

type RoundHealths []RoundHealth

type RoundHealth struct {
	SteamID                uint64
	MinHealthAboveZero     int
	PlayerWhoGetsTheDamage uint64
}

type KniferoundStats struct {
	SteamID uint64
	Kills   int
	Assists int
	Deaths  int
}

type Scoreboard struct {
	PlayerScores    []PlayerScore    `json:"player_scores"`
	RoundsPlayed    int              `json:"rounds_played"`
	TeamNames       map[int]string   `json:"team_names"`
	TeamMemebers    map[int][]uint64 `json:"team_members"`
	WinnerTeamID    int              `json:"winner_team_id"`
	WinnerTeam      string           `json:"winner_team"`
	KDTypeBits      map[int]string   `json:"kd_type_bits"`
	MaxRounds       int              `json:"max_rounds"`
	MapName         string           `json:"map_name"`
	knifeRoundMatch bool
	teamsSwapped    bool
}

type PlayerScore struct {
	// General stats
	SteamID            uint64         `json:"steam_id"`
	Nickname           string         `json:"nickname"`
	Kills              int            `json:"kills"`
	Assists            int            `json:"assists"`
	Deaths             int            `json:"deaths"`
	Kast               float64        `json:"kast"`
	DamageDone         int            `json:"damage_done"`
	DamageReceived     int            `json:"damage_received"`
	TeamDamageDone     int            `json:"team_damage_done"`
	TeamDamageReceived int            `json:"team_damage_received"`
	ADR                float64        `json:"adr"`
	Mvps               int            `json:"mvps"`
	MoneySpentTotal    int            `json:"money_spent_total"`
	KillsByWeapon      map[string]int `json:"kills_by_weapon"`
	KillsByType        map[uint32]int `json:"kills_by_type"`
	DeathsByWeapon     map[string]int `json:"deaths_by_weapon"`
	DeathsByType       map[uint32]int `json:"deaths_by_type"`
	ChickenKills       int            `json:"chicken_kills"`
	PlayedRounds       int            `json:"played_rounds"`
	playerRef          *common.Player

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

	// Team
	Team       string `json:"team"`
	TeamId     int    `json:"team_id"`
	TeamRounds int    `json:"team_rounds"`

	// Utility stats
	HeDamageDealt        int `json:"he_damage_dealt"`
	HeDamageReceived     int `json:"he_damage_received"`
	TeamHeDamageDealt    int `json:"team_he_damage_dealt"`
	TeamHeDamageReceived int `json:"team_he_damage_received"`
	HeSelfDamage         int `json:"he_self_damage"`
	HesThrown            int `json:"hes_thrown"`

	BurnDamageDealt        int `json:"burn_damage_dealt"`
	BurnDamageReceived     int `json:"burn_damage_received"`
	TeamBurnDamageDealt    int `json:"team_burn_damage_dealt"`
	TeamBurnDamageReceived int `json:"team_burn_damage_received"`
	BurnSelfDamage         int `json:"burn_self_damage"`
	BurnsThrown            int `json:"burns_thrown"`

	EnemiesFullFlashed      int `json:"enemies_full_flashed"`
	FullFlashesReceived     int `json:"full_flashes_received"`
	TeammatesFullFlashed    int `json:"team_full_flashes"`
	TeamFullFlashesReceived int `json:"team_full_flashes_received"`
	SelfFullFlashes         int `json:"self_full_flashes"`
	EnemiesHalfFlashed      int `json:"enemies_half_flashed"`
	HalfFlashesReceived     int `json:"half_flashes_received"`
	TeammatesHalfFlashed    int `json:"team_half_flashes"`
	TeamHalfFlashesReceived int `json:"team_half_flashes_received"`
	SelfHalfFlashes         int `json:"self_half_flashes"`
	FlashesThrown           int `json:"flashes_thrown"`

	SmokesThrown int `json:"smokes_thrown"`
	DecoysThrown int `json:"decoys_thrown"`

	// Kill and death stats
	HeadshotKills      int `json:"headshot_kills"`
	HeadshotDeaths     int `json:"headshot_deaths"`
	TeamHeadshotKills  int `json:"team_headshot_kills"`
	TeamHeadshotDeaths int `json:"team_headshot_deaths"`

	SmokeKills      int `json:"smoke_kills"`
	SmokeDeaths     int `json:"smoke_deaths"`
	TeamSmokeKills  int `json:"team_smoke_kills"`
	TeamSmokeDeaths int `json:"team_smoke_deaths"`

	BlindKills      int `json:"blind_kills"`
	BlindDeaths     int `json:"blind_deaths"`
	TeamBlindKills  int `json:"team_blind_kills"`
	TeamBlindDeaths int `json:"team_blind_deaths"`

	FlashAssists     int `json:"flash_assists"`
	FlashKills       int `json:"flash_kills"`
	FlashDeaths      int `json:"flash_deaths"`
	TeamFlashAssists int `json:"team_flash_assists"`
	TeamFlashKills   int `json:"team_flash_kills"`
	TeamFlashDeaths  int `json:"team_flash_deaths"`

	NoscopeKills      int `json:"no_scope_kills"`
	NoscopeDeaths     int `json:"no_scope_deaths"`
	TeamNoscopeKills  int `json:"team_no_scope_kills"`
	TeamNoscopeDeaths int `json:"team_no_scope_deaths"`

	WallBangKills      int `json:"wallbang_kills"`
	WallBangDeaths     int `json:"wallbang_deaths"`
	TeamWallBangKills  int `json:"team_wallbang_kills"`
	TeamWallBangDeaths int `json:"team_wallbang_deaths"`

	Suicides int `json:"suicides"`

	Reloads          int `json:"reloads"`
	ShotsFired       int `json:"shots_fired"`
	ShotsOnEnemies   int `json:"shots_on_enemies"`
	ShotsOnTeammates int `json:"shots_on_teammates"`
	Enemy2k          int `json:"enemy_2k"`
	Enemy3k          int `json:"enemy_3k"`
	Enemy4k          int `json:"enemy_4k"`
	Enemy5k          int `json:"enemy_5k"`
	EntryCount       int `json:"entry_count"`
	EntryWins        int `json:"entry_wins"`

	ClutchV1Count int `json:"clutch_v1_count"`
	ClutchV1Wins  int `json:"clutch_v1_wins"`
	ClutchV2Count int `json:"clutch_v2_count"`
	ClutchV2Wins  int `json:"clutch_v2_wins"`
	ClutchV3Count int `json:"clutch_v3_count"`
	ClutchV3Wins  int `json:"clutch_v3_wins"`
	ClutchV4Count int `json:"clutch_v4_count"`
	ClutchV4Wins  int `json:"clutch_v4_wins"`
	ClutchV5Count int `json:"clutch_v5_count"`
	ClutchV5Wins  int `json:"clutch_v5_wins"`

	KnifeRoundKills   int `json:"kniferound_kills"`
	KnifeRoundAssists int `json:"kniferound_assists"`
	KnifeRoundDeaths  int `json:"kniferound_deaths"`
	/////////////////////////////////////////////////

	OnDeathDroppedUtilityValue       int
	OnDeathDroppedBoughtUtilityValue int

	// Determining if something was bough wasn't trivial knowledge to dig out, so these are still waiting
	// FlashesBought  int // itemppickup
	// FlashesDropped int // itemdropped

	// HesBought  int
	// HesDropped int

	// BurnsBought  int
	// BurnsDropped int

	// SmokesBought  int
	// SmokesDropped int

	// DecoysBought  int
	// DecoysDropped int
}

package gamereport

type GameReport struct {
	JSONversion int `json:"JSONversion"`
	Game        struct {
		AlliancesType  int    `json:"alliancesType"`
		BaseType       int    `json:"baseType"`
		GameLimit      int    `json:"gameLimit"`
		IdleTime       int    `json:"idleTime"`
		MapName        string `json:"mapName"`
		MaxPlayers     int    `json:"maxPlayers"`
		Mods           string `json:"mods"`
		MultiTechLevel int    `json:"multiTechLevel"`
		PowerType      int    `json:"powerType"`
		Scavengers     int    `json:"scavengers"`
		StartDate      int64  `json:"startDate"`
		Version        string `json:"version"`
	} `json:"game"`
	GameTime   int                    `json:"gameTime"`
	PlayerData []GameReportPlayerData `json:"playerData"`
}

type GameReportExtended struct {
	JSONversion int   `json:"JSONversion"`
	EndDate     int64 `json:"endDate"`
	Game        struct {
		AlliancesType  int    `json:"alliancesType"`
		BaseType       int    `json:"baseType"`
		GameLimit      int    `json:"gameLimit"`
		IdleTime       int    `json:"idleTime"`
		MapName        string `json:"mapName"`
		MaxPlayers     int    `json:"maxPlayers"`
		Mods           string `json:"mods"`
		MultiTechLevel int    `json:"multiTechLevel"`
		PowerType      int    `json:"powerType"`
		Scavengers     int    `json:"scavengers"`
		StartDate      int64  `json:"startDate"`
		TimeGameEnd    int    `json:"timeGameEnd"`
		Timeout        bool   `json:"timeout"`
		Version        string `json:"version"`
	} `json:"game"`
	GameTime         int                    `json:"gameTime"`
	PlayerData       []GameReportPlayerData `json:"playerData"`
	ResearchComplete []struct {
		Name     string `json:"name"`
		Position int    `json:"position"`
		Struct   int    `json:"struct"`
		Time     int    `json:"time"`
	} `json:"researchComplete"`
}

type GameReportGraphFrame struct {
	GameTime int `json:"gameTime"`

	Kills                     []int `json:"kills"`
	Power                     []int `json:"power"`
	Score                     []int `json:"score"`
	Droids                    []int `json:"droids"`
	DroidsBuilt               []int `json:"droidsBuilt"`
	DroidsLost                []int `json:"droidsLost"`
	Hp                        []int `json:"hp"`
	Structs                   []int `json:"structs"`
	StructuresBuilt           []int `json:"structuresBuilt"`
	StructuresLost            []int `json:"structuresLost"`
	StructureKills            []int `json:"structureKills"`
	SummExp                   []int `json:"summExp"`
	OilRigs                   []int `json:"oilRigs"`
	ResearchComplete          []int `json:"researchComplete"`
	RecentPowerLost           []int `json:"recentPowerLost"`
	RecentPowerWon            []int `json:"recentPowerWon"`
	RecentResearchPerformance []int `json:"recentResearchPerformance"`
	RecentResearchPotential   []int `json:"recentResearchPotential"`

	RecentDroidPowerLost     []int `json:"recentDroidPowerLost"`
	RecentStructurePowerLost []int `json:"recentStructurePowerLost"`
}

type GameReportPlayerStatistics struct {
	Kills                     int `json:"kills"`
	Power                     int `json:"power"`
	Score                     int `json:"score"`
	Droids                    int `json:"droids"`
	DroidsBuilt               int `json:"droidsBuilt"`
	DroidsLost                int `json:"droidsLost"`
	Hp                        int `json:"hp"`
	Structs                   int `json:"structs"`
	StructuresBuilt           int `json:"structuresBuilt"`
	StructuresLost            int `json:"structuresLost"`
	StructureKills            int `json:"structureKills"`
	SummExp                   int `json:"summExp"`
	OilRigs                   int `json:"oilRigs"`
	ResearchComplete          int `json:"researchComplete"`
	RecentPowerLost           int `json:"recentPowerLost"`
	RecentPowerWon            int `json:"recentPowerWon"`
	RecentResearchPerformance int `json:"recentResearchPerformance"`
	RecentResearchPotential   int `json:"recentResearchPotential"`

	RecentDroidPowerLost     int `json:"recentDroidPowerLost"`
	RecentStructurePowerLost int `json:"recentStructurePowerLost"`
}

type GameReportPlayerData struct {
	Index     int    `json:"index"`
	Position  int    `json:"position"`
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	Team      int    `json:"team"`
	Usertype  string `json:"usertype"`
	Color     int    `json:"colour"`
	Faction   int    `json:"faction"`
	GameReportPlayerStatistics
}

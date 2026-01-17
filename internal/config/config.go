package config

type Config struct {
	Languages	string
	StagedOnly	bool
	MaxFiles	int
	Format		string
}


type ReviewOptions struct {
	Languages	[]string
	StagedOnly	bool
	MaxFiles	int
	Format		string
	Interactive	bool
}
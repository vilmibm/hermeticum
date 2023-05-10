package witch

const (
	dirEast  = "_DIR_EAST"
	dirWest  = "_DIR_WEST"
	dirNorth = "_DIR_NORTH"
	dirSouth = "_DIR_SOUTH"
	dirAbove = "_DIR_ABOVE"
	dirBelow = "_DIR_BELOW"
)

type Direction struct {
	raw string
}

func (d Direction) Reverse() Direction {
	raw := ""
	switch d.raw {
	case dirAbove:
		raw = dirBelow
	case dirBelow:
		raw = dirAbove
	case dirEast:
		raw = dirWest
	case dirWest:
		raw = dirEast
	case dirNorth:
		raw = dirSouth
	case dirSouth:
		raw = dirNorth
	}
	return Direction{raw: raw}
}

// NormalizeHuman takes a direction someone might type like "up" or "north" and returns the correct Direction struct
func NormalizeHuman(humanDir string) Direction {
	raw := ""
	switch humanDir {
	case "up":
	case "above":
		raw = dirAbove
	case "down":
	case "below":
		raw = dirBelow
	case "east":
		raw = dirEast
	case "west":
		raw = dirWest
	case "north":
		raw = dirNorth
	case "south":
		raw = dirSouth

	}
	return Direction{raw: raw}
}

// Human returns a string form of this direction like "above" or "north"
func (d Direction) Human() (humanDir string) {
	switch d.raw {
	case dirAbove:
		humanDir = "above"
	case dirBelow:
		humanDir = "below"
	case dirEast:
		humanDir = "east"
	case dirWest:
		humanDir = "west"
	case dirNorth:
		humanDir = "north"
	case dirSouth:
		humanDir = "south"
	}

	return humanDir
}

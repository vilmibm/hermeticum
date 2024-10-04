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

func newDirection(raw string) Direction {
	return Direction{raw: raw}
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
	return newDirection(raw)
}

// NormalizeDirection takes a direction someone might type like "up" or "north" and returns the correct Direction struct
func NormalizeDirection(humanDir string) Direction {
	raw := ""
	switch humanDir {
	case "up":
		raw = dirAbove
	case "above":
		raw = dirAbove
	case "down":
		raw = dirBelow
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
	default:
		panic("invalid direction")
	}

	return newDirection(raw)
}

func Directions() []string {
	return []string{
		"up",
		"above",
		"down",
		"below",
		"east",
		"west",
		"north",
		"south",
	}
}

func ValidDirection(input string) bool {
	switch input {
	case "up":
		return true
	case "above":
		return true
	case "down":
		return true
	case "below":
		return true
	case "east":
		return true
	case "west":
		return true
	case "north":
		return true
	case "south":
		return true
	default:
		return false
	}
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

func (d Direction) IsVertical() bool {
	return d.raw == dirAbove || d.raw == dirBelow
}

func (d Direction) Equals(o Direction) bool {
	return d.raw == o.raw
}

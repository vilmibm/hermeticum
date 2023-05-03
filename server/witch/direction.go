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
	// TODO
	return Direction{}
}

// NormalizeHuman takes a direction someone might type like "up" or "north" and returns the correct Direction struct
func NormalizeHuman(humanDir string) Direction {
	// TODO
	return Direction{}
}

// Human returns a string form of this direction like "above" or "north"
func (d Direction) Human() string {
	// TODO
	return ""
}

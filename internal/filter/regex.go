package filter

import "regexp"

// Regex returns a line filter that matches lines against the given pattern.
// If invert is true, non-matching lines are kept instead.
func Regex(pattern string, invert bool) (func(string) bool, error) {
	if pattern == "" {
		return nil, nil // no filter
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return func(line string) bool {
		matched := re.MatchString(line)
		if invert {
			return !matched
		}
		return matched
	}, nil
}

// Package join implements a generic interface for matching elements from two slices
// similar in spirit to a database Join.
package join

import "reflect"

// A Pair represents an element from the left slice and an element from the right slice,
// that have been matched by a join.
type Pair struct {
	L, R interface{}
}

// Join attempts to match each element in `lSlice` with an element in `rSlice` in
// accordance with a score function.  If such a match is found, it is returned as an
// element of `pairs`, while leftover elements from `lSlice` and `rSlice` that couldn`t
// be matched, are returned as elements of `lonelyLefts` and `lonelyRights`
// respectively. Both `lSlice` and `rSlice` must be slice or array types, but they do
// not necessarily have to have the same type.
//
// Matches are made in accordance with the provided `score` function.  It takes a single
// element from `lSlice`, and a single element from `rSlice`, and computes a score
// suggesting their match preference.  The algorithm prefers to match pairs with the
// the score closest to zero (inclusive). Negative scores are never matched.
func Join(lSlice, rSlice interface{}, score func(left, right interface{}) int) (
	pairs []Pair, lonelyLefts, lonelyRights []interface{}) {

	val := reflect.ValueOf(rSlice)
	len := val.Len()
	lonelyRights = make([]interface{}, 0, len)

	for i := 0; i < len; i++ {
		lonelyRights = append(lonelyRights, val.Index(i).Interface())
	}

	val = reflect.ValueOf(lSlice)
	len = val.Len()

Outer:
	for i := 0; i < len; i++ {
		l := val.Index(i).Interface()
		bestScore := -1
		bestIndex := -1
		for i, r := range lonelyRights {
			s := score(l, r)
			switch {
			case s < 0:
				continue
			case s == 0:
				pairs = append(pairs, Pair{l, r})
				lonelyRights = sliceDel(lonelyRights, i)
				continue Outer
			case s < bestScore || bestScore < 0:
				bestIndex = i
				bestScore = s
			}
		}

		if bestIndex >= 0 {
			pairs = append(pairs, Pair{l, lonelyRights[bestIndex]})
			lonelyRights = sliceDel(lonelyRights, bestIndex)
			continue Outer
		}

		lonelyLefts = append(lonelyLefts, l)
	}

	return pairs, lonelyLefts, lonelyRights
}

func sliceDel(slice []interface{}, i int) []interface{} {
	l := len(slice)
	slice[i] = slice[l-1]
	slice[l-1] = nil // Allow garbage collection.
	return slice[:l-1]
}

// List simply requires implementing types to allow access to their contained values by
// integer index.
type List interface {
	Len() int
	Get(int) interface{}
}

// HashJoin attempts to match each element in `lSlice` with an element in `rSlice` by
// performing a hash join. If such a match is found for a given element of `lSlice`,
// it is returned as an element of `pairs`, while leftover elements from `lSlice` and
// `rSlice` that couldn`t be matched are returned as elements of `lonelyLefts` and
// `lonelyRights` respectively. The join keys for `lSlice` and `rSlice` are defined by
// the passed in `lKey` and `rKey` functions, respectively.
//
// If `lKey` or `rKey` are nil, the elements of the respective slices are used directly
// as keys instead.
func HashJoin(lSlice, rSlice List, lKey, rKey func(interface{}) interface{}) (
	pairs []Pair, lonelyLefts, lonelyRights []interface{}) {

	// If the rSlice is shorter than the lSlice, use the rSlice to create the
	// reference hash table in order to save memory.
	if rSlice.Len() < lSlice.Len() {
		// Call hashJoin with swapped arguments, and swap the result back to
		// the order the caller is expecting.
		pairs, lonelyRights, lonelyLefts = hashJoin(rSlice, lSlice, rKey, lKey)
		for i, p := range pairs {
			pairs[i] = Pair{p.R, p.L}
		}
	} else {
		pairs, lonelyLefts, lonelyRights = hashJoin(lSlice, rSlice, lKey, rKey)
	}
	return pairs, lonelyLefts, lonelyRights
}

func hashJoin(lSlice, rSlice List, lKey, rKey func(interface{}) interface{}) (
	pairs []Pair, lonelyLefts, lonelyRights []interface{}) {

	var identity = func(val interface{}) interface{} {
		return val
	}

	if lKey == nil {
		lKey = identity
	}
	if rKey == nil {
		rKey = identity
	}

	// lonely lefts are tracked implicitly by remaining elements in joinTable
	joinTable := make(map[interface{}]*interface{})

	for ii := 0; ii < lSlice.Len(); ii++ {
		lElem := lSlice.Get(ii)
		joinTable[lKey(lElem)] = &lElem
	}

	// Query the join table and match pairs using rSlice.
	// As matches are found, remove from lonely lefts.
	// As matches are not found, add to lonely rights.
	for ii := 0; ii < rSlice.Len(); ii++ {
		rElem := rSlice.Get(ii)
		rElemKey := rKey(rElem)
		if entry, ok := joinTable[rElemKey]; ok {
			pairs = append(pairs, Pair{*entry, rElem})
			delete(joinTable, rElemKey) // ok since rElemKey == lElemKey here
		} else {
			lonelyRights = append(lonelyRights, rElem)
		}
	}

	// transform the lonely sets back into slices (note: random order!)
	for _, ll := range joinTable {
		lonelyLefts = append(lonelyLefts, *ll)
	}

	return pairs, lonelyLefts, lonelyRights
}

// StringSlice is an alias for []string to allow for joins
type StringSlice []string

// Get returns the value contained at the given index
func (ss StringSlice) Get(ii int) interface{} {
	return ss[ii]
}

// Len returns the number of items in the slice
func (ss StringSlice) Len() int {
	return len(ss)
}

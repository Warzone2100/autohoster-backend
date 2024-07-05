package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maxsupermanhd/lac/v2"
)

var (
	randomsource = rand.NewSource(time.Now().Unix())
)

func tryCfgGetD[T any](f func(cfg lac.Conf) (T, bool), d T, cfgs ...lac.Conf) T {
	r := tryCfgGet(f, cfgs...)
	if r == nil {
		return d
	}
	return *r
}

func tryCfgGet[T any](f func(cfg lac.Conf) (T, bool), cfgs ...lac.Conf) *T {
	for _, v := range cfgs {
		ret, ok := f(v)
		if ok {
			return &ret
		}
	}
	return nil
}

func tryGetSliceStringGen(k ...string) func(c lac.Conf) ([]string, bool) {
	return func(c lac.Conf) ([]string, bool) {
		return c.GetSliceString(k...)
	}
}

func tryGetSliceIntGen(k ...string) func(c lac.Conf) ([]int, bool) {
	return func(c lac.Conf) ([]int, bool) {
		return c.GetSliceInt(k...)
	}
}

func tryGetBoolGen(k ...string) func(c lac.Conf) (bool, bool) {
	return func(c lac.Conf) (bool, bool) {
		return c.GetBool(k...)
	}
}

func tryGetIntGen(k ...string) func(c lac.Conf) (int, bool) {
	return func(c lac.Conf) (int, bool) {
		return c.GetInt(k...)
	}
}

func tryGetStringGen(k ...string) func(c lac.Conf) (string, bool) {
	return func(c lac.Conf) (string, bool) {
		return c.GetString(k...)
	}
}

func pickNumberD(c lac.Conf, d int, k ...string) int {
	v, ok := pickNumber(c, k...)
	if !ok {
		return d
	}
	return v
}

func tryPickNumberGen(k ...string) func(c lac.Conf) (int, bool) {
	return func(c lac.Conf) (int, bool) {
		return pickNumber(c, k...)
	}
}

func pickNumber(c lac.Conf, k ...string) (int, bool) {
	s, ok := c.GetString(k...)
	if !ok {
		return 0, false
	}
	valsS := strings.Split(s, ",")
	vals := []int{}
	for i, vs := range valsS {
		v, err := strconv.Atoi(vs)
		if err != nil {
			log.Printf("Failed to parse number in number pick string %q pos %d", s, i)
			continue
		}
		vals = append(vals, v)
	}
	if len(vals) == 0 {
		return 0, false
	}
	return vals[int(randomsource.Int63())%len(vals)], true
}

// parses string into numbers, can have ranges with dashes, example "23,31,90-93"
// only positive integers supported
func parseNumbersString(input string) []int {
	numbers := []int{}
	for i, v := range strings.Split(input, ",") {
		vv := strings.Split(v, "-")
		if len(vv) == 1 {
			p, err := strconv.Atoi(vv[0])
			if err != nil {
				log.Printf("Failed to parse port string %q at region %v (%q): %s", input, i, vv[0], err.Error())
				continue
			}
			numbers = append(numbers, p)
		} else if len(vv) == 2 {
			pBegin, err := strconv.Atoi(vv[0])
			if err != nil {
				log.Printf("Failed to parse port string %q at region %v (%q): %s", input, i, vv[0], err.Error())
				continue
			}
			pEnd, err := strconv.Atoi(vv[1])
			if err != nil {
				log.Printf("Failed to parse port string %q at region %v (%q): %s", input, i, vv[0], err.Error())
				continue
			}
			for p := pBegin; p <= pEnd; p++ {
				numbers = append(numbers, p)
			}
		} else {
			log.Printf("Weird port entry you have here: %q", v)
		}
	}
	return numbers
}

// https://stackoverflow.com/questions/66643946/how-to-remove-duplicates-strings-or-int-from-slice-in-go
func removeDuplicate[T comparable](sliceList []T) []T {
	allKeys := make(map[T]bool)
	list := []T{}
	for _, item := range sliceList {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

func genRandomString(l int) string {
	chars := "abcdefghigklmnopqrstwxyzABCDEFGHIGKLMNOPQRSTWXYZ0123456789"
	ret := ""
	for i := 0; i < l; i++ {
		ret += string(chars[rand.Intn(len(chars))])
	}
	return ret
}

func makeDirs(perm os.FileMode, dirs []string) error {
	for _, v := range dirs {
		err := os.MkdirAll(v, perm)
		if err != nil {
			return err
		}
	}
	return nil
}

func stringContainsSlices(str string, sl []string) bool {
	for _, v := range sl {
		if strings.Contains(str, v) {
			return true
		}
	}
	return false
}

func base64DecodeFields(vs ...any) error {
	if len(vs) == 0 {
		return nil
	}
	if len(vs)%2 == 1 {
		return fmt.Errorf("wrong number of arguments provided: %d", len(vs))
	}
	for i := 0; i < len(vs); i += 2 {
		df, ok := vs[i].(string)
		if !ok {
			return fmt.Errorf("argument %d should be of type string but it is of type %t", i, vs[i])
		}
		dt, ok := vs[i+1].(*[]byte)
		if !ok {
			return fmt.Errorf("argument %d should be of type string but it is of type %t", i+1, vs[i+1])
		}
		if dt == nil {
			return fmt.Errorf("argument %d is nil", i+1)
		}
		var err error
		*dt, err = base64.StdEncoding.DecodeString(df)
		if err != nil {
			return err
		}
	}
	return nil
}

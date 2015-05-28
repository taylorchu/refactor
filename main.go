package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Author struct {
	Name  string
	Email string
	Time  time.Time
}

type Diff struct {
	File   string
	Add    int
	Delete int
}

type Commit struct {
	ID      string
	Tree    string
	Parent  string
	Author  Author
	Message []string
	Diff    []Diff
}

var (
	commitRegexp  = regexp.MustCompile(`^commit (.+)$`)
	treeRegexp    = regexp.MustCompile(`^tree (.+)$`)
	parentRegexp  = regexp.MustCompile(`^parent (.+)$`)
	authorRegexp  = regexp.MustCompile(`^author ([^ ]+) <([^ ]+)> ([^ ]+) [^ ]+$`)
	messageRegexp = regexp.MustCompile(`^[ ]{4}(.+)$`)
	diffRegexp    = regexp.MustCompile(`^([0-9]+)\t([0-9]+)\t(.+)$`)
)

func GitLog() (commits []*Commit, err error) {
	b, err := exec.Command("git", "log", "--all",
		fmt.Sprintf(`--after="%s"`, *after),
		fmt.Sprintf(`--before="%s"`, *before),
		"--format=raw", "--numstat").Output()
	if err != nil {
		return
	}

	s := bufio.NewScanner(bytes.NewReader(b))
	for s.Scan() {
		line := s.Text()
		if match := commitRegexp.FindStringSubmatch(line); match != nil {
			commits = append(commits, &Commit{
				ID: match[1],
			})
		} else if match := treeRegexp.FindStringSubmatch(line); match != nil {
			if len(commits) == 0 {
				continue
			}
			commits[len(commits)-1].Tree = match[1]
		} else if match := parentRegexp.FindStringSubmatch(line); match != nil {
			if len(commits) == 0 {
				continue
			}
			commits[len(commits)-1].Parent = match[1]
		} else if match := authorRegexp.FindStringSubmatch(line); match != nil {
			if len(commits) == 0 {
				continue
			}
			i, err := strconv.ParseInt(match[3], 10, 64)
			if err != nil {
				continue
			}
			commits[len(commits)-1].Author = Author{
				Name:  match[1],
				Email: match[2],
				Time:  time.Unix(i, 0),
			}
		} else if match := messageRegexp.FindStringSubmatch(line); match != nil {
			if len(commits) == 0 {
				continue
			}
			commits[len(commits)-1].Message = append(commits[len(commits)-1].Message, match[1])
		} else if match := diffRegexp.FindStringSubmatch(line); match != nil {
			if len(commits) == 0 {
				continue
			}
			add, err := strconv.ParseInt(match[1], 10, 64)
			if err != nil {
				continue
			}
			del, err := strconv.ParseInt(match[2], 10, 64)
			if err != nil {
				continue
			}
			commits[len(commits)-1].Diff = append(commits[len(commits)-1].Diff, Diff{
				Add:    int(add),
				Delete: int(del),
				File:   match[3],
			})
		}
	}
	return
}

var (
	addRegexp        = regexp.MustCompile(`^\+([^+].*)$`)
	delRegexp        = regexp.MustCompile(`^\-([^-].*)$`)
	usefulLineRegexp = regexp.MustCompile(`(?:[a-zA-Z0-9_]+\(|^if |^for |=)`)
)

func GitDiff(commitID string) (add, del []string, err error) {
	b, err := exec.Command("git", "diff", commitID+"^!").Output()
	if err != nil {
		return
	}
	s := bufio.NewScanner(bytes.NewReader(b))
	for s.Scan() {
		line := s.Text()
		if match := addRegexp.FindStringSubmatch(line); match != nil {
			s := strings.TrimSpace(match[1])
			// ignore comments
			if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "*") {
				continue
			}
			if !usefulLineRegexp.MatchString(s) {
				continue
			}
			add = append(add, s)
		} else if match := delRegexp.FindStringSubmatch(line); match != nil {
			s := strings.TrimSpace(match[1])
			// ignore comments
			if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "*") {
				continue
			}
			if !usefulLineRegexp.MatchString(s) {
				continue
			}
			del = append(del, s)
		}
	}
	return
}

type Reason struct {
	Line  string
	Count int
}

type ByCount []*Reason

func (s ByCount) Len() int      { return len(s) }
func (s ByCount) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s ByCount) Less(i, j int) bool {
	return s[i].Count > s[j].Count
}

type Target struct {
	Name   string
	Commit []*Commit
	Score  float64
	Reason []*Reason
}

func edit2score(n int) (score float64) {
	for {
		if n < 1 {
			break
		}
		score++
		n /= 10
	}
	return
}

type ByScore []*Target

func (s ByScore) Len() int      { return len(s) }
func (s ByScore) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s ByScore) Less(i, j int) bool {
	return s[i].Score > s[j].Score ||
		s[i].Score == s[j].Score && len(s[i].Commit) > len(s[j].Commit)
}

var (
	after     = flag.String("after", "1 week ago", "inspect commits after that time")
	before    = flag.String("before", time.Now().Format(time.RFC3339), "inspect commits before that time")
	topTarget = flag.Int("target", 10, "show top K targets")
	topReason = flag.Int("reason", 3, "show top K reasons")
	detail    = flag.Bool("detail", false, "show reason with only 1 count")
)

func main() {
	flag.Parse()

	commits, err := GitLog()
	if err != nil {
		return
	}
	m := make(map[string]*Target)
	add := func(name string, commit *Commit, score float64) {
		if t, ok := m[name]; ok {
			t.Commit = append(t.Commit, commit)
			t.Score += score
		} else {
			m[name] = &Target{
				Name:   name,
				Score:  score,
				Commit: []*Commit{commit},
			}
		}
	}
	for _, commit := range commits {
		var files []string
		var score float64
		for _, diff := range commit.Diff {
			// per-file
			if strings.HasSuffix(diff.File, ".h") ||
				strings.HasSuffix(diff.File, ".c") ||
				strings.HasSuffix(diff.File, ".go") {

				fileScore := edit2score(diff.Add + diff.Delete)

				// update group entry
				files = append(files, diff.File)
				score += fileScore

				// update file entry
				add(diff.File, commit, fileScore)
			}
		}

		if len(files) >= 2 {
			score *= float64(len(files))
			// per-group
			group := strings.Join(files, ",")
			add(group, commit, score)
		}
	}

	// so far it calculates based on edit distance
	var targets []*Target
	for _, t := range m {
		// diff analysis
		plus := make(map[string]string)
		minus := make(map[string]string)
		delta := make(map[string]int)

		for _, commit := range t.Commit {
			add, del, err := GitDiff(commit.ID)
			if err != nil {
				continue
			}
			for _, line := range add {
				if id, ok := minus[line]; ok && id != commit.ID {
					delta[line]++
					delete(minus, line)
				}
				plus[line] = commit.ID
			}
			for _, line := range del {
				if id, ok := plus[line]; ok && id != commit.ID {
					delta[line]++
					delete(plus, line)
				}
				minus[line] = commit.ID
			}
		}
		var total int
		for line, count := range delta {
			t.Reason = append(t.Reason, &Reason{
				Line:  line,
				Count: count,
			})
			total += count
		}
		sort.Sort(ByCount(t.Reason))
		t.Score *= float64(total)
		if t.Score > 0 {
			targets = append(targets, t)
		}
	}
	// sort this list
	sort.Sort(ByScore(targets))

	// top K
	for i, t := range targets {
		if i == *topTarget {
			break
		}
		fmt.Printf("%8.1f %-40s %4d\n",
			t.Score,
			shorten(t.Name, 40),
			len(t.Commit),
		)
		for i, reason := range t.Reason {
			if i == *topReason {
				break
			}
			if *detail || reason.Count > 1 {
				fmt.Printf("    %4d %s\n", reason.Count, reason.Line)
			}
		}
		if *detail {
			for _, commit := range t.Commit {
				var msg string
				if len(commit.Message) > 0 {
					msg = commit.Message[0]
				}
				fmt.Printf("         %s %s (%s)\n",
					commit.ID[:7],
					msg,
					commit.Author.Name,
				)
			}
		}
		fmt.Println()
	}
	fmt.Printf("total targets: %d, total commits: %d\n", len(targets), len(commits))
}

func shorten(s string, l int) string {
	if l < 3 {
		return ""
	}
	if len(s) > l {
		return s[:l-3] + "..."
	}
	return s
}

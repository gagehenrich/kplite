package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/rthornton128/goncurses"
	"github.com/tobischo/gokeepasslib/v3"
	"golang.org/x/term"
)

type Entry struct {
	Title    string
	Username string
	Password string
	URL      string
	Notes    string
}

type GroupNode struct {
	Name      string
	Entries   []Entry
	SubGroups []*GroupNode
	Expanded  bool
	Parent    *GroupNode
	Index     int 				// track absolute group number
}

type VisibleItem struct {
	Group    *GroupNode
	Depth    int
	Position int
}

type ViewState struct {
	selectedIndex  int
	entryScrollPos int
	groupScrollPos int
	searchQuery    string
	inSearchMode   bool
	showPasswords  bool
	focusedPane    int 			// 0=groups, 1=entries
}

var expandAllGroups = false


func getPassword() (string, error) {
	fmt.Print("Enter database password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}

func getVisibleItems(root *GroupNode) []VisibleItem {
	var items []VisibleItem
	addVisibleItems(root, 0, &items)
	return items
}

func addVisibleItems(group *GroupNode, depth int, items *[]VisibleItem) {
	if group == nil {
		return
	}

	*items = append(*items, VisibleItem{
		Group:    group,
		Depth:    depth,
		Position: len(*items),
	})

	if group.Expanded {
		for _, subGroup := range group.SubGroups {
			addVisibleItems(subGroup, depth+1, items)
		}
	}
}

func displayGroups(win *goncurses.Window, items []VisibleItem, selectedIndex int, scrollPos int) {
	maxY, maxX := win.MaxYX()
	if maxY <= 1 {
		return
	}

	// Calculate visible range
	visibleLines := maxY - 2 // Account for border
	startIdx := scrollPos
	endIdx := startIdx + visibleLines
	if endIdx > len(items) {
		endIdx = len(items)
	}

	// Display scroll indicators if needed
	if scrollPos > 0 {
		win.MovePrint(1, maxX-2, "↑")
	}
	if endIdx < len(items) {
		win.MovePrint(maxY-2, maxX-2, "↓")
	}

	// Display visible items
	for i, item := range items[startIdx:endIdx] {
		displayY := i + 1 // Start at line 1 to account for border
		prefix := strings.Repeat("  ", item.Depth)
		indicator := "-"
		if len(item.Group.SubGroups) > 0 {
			if item.Group.Expanded {
				indicator = "+"
			} else {
				indicator = ">"
			}
		}

		displayText := fmt.Sprintf("%s%s %s", prefix, indicator, item.Group.Name)
		if item.Position == selectedIndex {
			win.AttrOn(goncurses.ColorPair(1))
			win.MovePrint(displayY, 1, fmt.Sprintf("%-*s", maxX-3, displayText))
			win.AttrOff(goncurses.ColorPair(1))
		} else {
			win.MovePrint(displayY, 1, fmt.Sprintf("%-*s", maxX-3, displayText))
		}
	}
}

func displayEntries(win *goncurses.Window, entries []Entry, width int, scrollPos int, showPasswords bool) {
	maxY, _ := win.MaxYX()
	entriesPerPage := (maxY - 2) / 4
	startIdx := scrollPos
	endIdx := startIdx + entriesPerPage
	if endIdx > len(entries) {
		endIdx = len(entries)
	}

	y := 1
	for _, entry := range entries[startIdx:endIdx] {
		if y >= maxY-1 {
			break
		}
		win.MovePrint(y, 1, fmt.Sprintf("Title: %s", truncateString(entry.Title, width-8)))
		y++
		win.MovePrint(y, 1, fmt.Sprintf("Username: %s", truncateString(entry.Username, width-11)))
		y++
		password := strings.Repeat("*", len(entry.Password))
		if showPasswords {
			password = entry.Password
		}
		win.MovePrint(y, 1, fmt.Sprintf("Password: %s", password))
		y++
		if entry.URL != "" {
			win.MovePrint(y, 1, fmt.Sprintf("URL: %s", truncateString(entry.URL, width-6)))
			y++
		}
		win.MovePrint(y, 1, strings.Repeat("-", width-2))
		y++
	}

	if len(entries) > entriesPerPage {
		if scrollPos > 0 {
			win.MovePrint(1, width-1, "↑")
		}
		if endIdx < len(entries) {
			win.MovePrint(maxY-2, width-1, "↓")
		}
	}
}

func expandCollapseGroups(group *GroupNode) {
	if group == nil {
		return
	}

	group.Expanded = expandAllGroups

	for _, subGroup := range group.SubGroups {
		expandCollapseGroups(subGroup)
	}

}

func buildGroupHierarchy(group gokeepasslib.Group, groupCounter *int) *GroupNode {
	// Debug logging to see group structure
	//log.Printf("Building group: %s with %d entries and %d subgroups",
	//    group.Name, len(group.Entries), len(group.Groups))

	node := &GroupNode{
		Name:      group.Name,
		Expanded:  false,
		Entries:   make([]Entry, 0),
		SubGroups: make([]*GroupNode, 0),
		Index:     *groupCounter,
	}
	(*groupCounter)++

	// Process entries
	for _, entry := range group.Entries {
		node.Entries = append(node.Entries, Entry{
			Title:    entry.GetTitle(),
			Username: entry.GetContent("UserName"),
			Password: entry.GetPassword(),
			URL:      entry.GetContent("URL"),
			Notes:    entry.GetContent("Notes"),
		})
	}

	// Process subgroups
	for _, subGroup := range group.Groups {
		childNode := buildGroupHierarchy(subGroup, groupCounter)
		if childNode != nil {
			childNode.Parent = node
			node.SubGroups = append(node.SubGroups, childNode)
		}
	}

	return node
}

func getString(win *goncurses.Window, y, x, maxLen int) string {
	var sb strings.Builder
	for {
		ch := win.GetChar()
		if ch == '\n' || ch == '\r' {
			break
		}
		if ch == 127 || ch == 8 { // Backspace
			if sb.Len() > 0 {
				str := sb.String()
				sb.Reset()
				sb.WriteString(str[:len(str)-1])
				win.MovePrint(y, x, strings.Repeat(" ", maxLen))
				win.MovePrint(y, x, sb.String())
			}
		} else if sb.Len() < maxLen {
			sb.WriteRune(rune(ch))
			win.MovePrint(y, x, sb.String())
		}
		win.Refresh()
	}
	return sb.String()
}

func searchEntries(root *GroupNode, query string) []*GroupNode {
	query = strings.ToLower(query)
	var matchingGroups []*GroupNode

	var search func(*GroupNode)
	search = func(group *GroupNode) {
		// Check entries in current group
		for _, entry := range group.Entries {
			if strings.Contains(strings.ToLower(entry.Title), query) ||
				strings.Contains(strings.ToLower(entry.Username), query) ||
				strings.Contains(strings.ToLower(entry.URL), query) ||
				strings.Contains(strings.ToLower(entry.Notes), query) {
				matchingGroups = append(matchingGroups, group)
				break
			}
		}

		// Recursively search subgroups
		for _, subGroup := range group.SubGroups {
			search(subGroup)
		}
	}

	search(root)
	return matchingGroups
}

func searchGroups(root *GroupNode, query string) (*GroupNode, int) {
	expandCollapseGroups(root)
	query = strings.ToLower(query)

	var search func(*GroupNode) *GroupNode
	search = func(group *GroupNode) *GroupNode {
		if group == nil {
			return nil
		}

		if strings.Contains(strings.ToLower(group.Name), query) {
			return group
		}

		for _, subGroup := range group.SubGroups {
			found := search(subGroup)
			if found != nil {
				return found
			}
		}

		return nil
	}

	foundGroup := search(root)
	if foundGroup != nil {
		return foundGroup, foundGroup.Index
	}
	return nil, -1
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: kdbx-viewer <database.kdbx>")
		os.Exit(1)
	}

	password, err := getPassword()
	if err != nil {
		log.Fatal("Error reading password:", err)
	}

	file, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	db := gokeepasslib.NewDatabase()
	db.Credentials = gokeepasslib.NewPasswordCredentials(password)

	if err := gokeepasslib.NewDecoder(file).Decode(db); err != nil {
		log.Fatal("Failed to decode database: ", err)
	}

	db.UnlockProtectedEntries()

	rootGroup := &GroupNode{Name: "Root", Expanded: true}
	groupCounter := 0
	for _, group := range db.Content.Root.Groups {
		subGroup := buildGroupHierarchy(group, &groupCounter)
		subGroup.Parent = rootGroup
		rootGroup.SubGroups = append(rootGroup.SubGroups, subGroup)
	}

	stdscr, err := goncurses.Init()
	if err != nil {
		log.Fatal("failed to initialize ncurses:", err)
	}
	defer goncurses.End()

	goncurses.Raw(true)
	goncurses.Echo(false)
	goncurses.Cursor(0)
	stdscr.Keypad(true)

	if !goncurses.HasColors() {
		log.Fatal("Terminal does not support colors")
	}
	goncurses.StartColor()
	goncurses.InitPair(1, goncurses.C_WHITE, goncurses.C_BLUE)

	maxY, maxX := stdscr.MaxYX()
	listWidth := maxX / 3

	groupWin, err := goncurses.NewWindow(maxY-4, listWidth, 1, 0)
	if err != nil {
		log.Fatal("failed to create group window:", err)
	}

	detailWin, err := goncurses.NewWindow(maxY-4, maxX-listWidth-1, 1, listWidth+1)
	if err != nil {
		log.Fatal("failed to create detail window:", err)
	}

	searchWin, err := goncurses.NewWindow(3, maxX, maxY-3, 0)
	if err != nil {
		log.Fatal("failed to create search window:", err)
	}

	stdscr.MovePrint(0, 0, "kplite [kdbx viewer] - (q: quit, arrows: navigate, enter: expand/collapse)")
	stdscr.Refresh()

	state := ViewState{
		selectedIndex:  0,
		entryScrollPos: 0,
		groupScrollPos: 0,
		searchQuery:    "",
		inSearchMode:   false,
		showPasswords:  false,
		focusedPane:    0,
	}

	// Update header to show new commands
	stdscr.MovePrint(0, 0, "kplite [kdbx viewer] - (q: quit, [space]: toggle passwords, enter: expand/collapse current group, f: expand/collapse all)")
	stdscr.Refresh()

	// Main loop
	for {
		groupWin.Clear()
		detailWin.Clear()
		searchWin.Clear()

		groupWin.Box(0, 0)
		detailWin.Box(0, 0)
		searchWin.Box(0, 0)

		groupWin.MovePrint(0, 2, " Groups ")
		detailWin.MovePrint(0, 2, "")
		searchWin.MovePrint(0, 2, fmt.Sprintf(" Search: %s", state.searchQuery))

		visibleItems := getVisibleItems(rootGroup)
		displayGroups(groupWin, visibleItems, state.selectedIndex, state.groupScrollPos)

		var selectedGroup *GroupNode
		if state.selectedIndex >= 0 && state.selectedIndex < len(visibleItems) {
			selectedGroup = visibleItems[state.selectedIndex].Group
		}

		var entries []Entry
		if state.inSearchMode && state.searchQuery != "" {
			matchingGroups := searchEntries(rootGroup, state.searchQuery)
			if len(matchingGroups) > 0 {
				// Set the selected index to the first matching group
				state.selectedIndex = -1
				for i, item := range visibleItems {
					for _, group := range matchingGroups {
						if item.Group == group {
							state.selectedIndex = i
							break
						}
					}
					if state.selectedIndex != -1 {
						break
					}
				}

				// Expand all the parent groups of the first matching group
				currentGroup := matchingGroups[0]
				for currentGroup != nil {
					currentGroup.Expanded = true
					currentGroup = currentGroup.Parent
				}

				// Display the entries for the first matching group
				entries = matchingGroups[0].Entries
				state.entryScrollPos = 0
			} else {
				entries = []Entry{}
			}
		} else if selectedGroup != nil {
			entries = selectedGroup.Entries[state.entryScrollPos:]
		}

		detailWin.MovePrint(0, 2, " ", selectedGroup.Name, " ")

		if state.focusedPane == 1 {
			detailWin.AttrOn(goncurses.ColorPair(1))
		} else {
			groupWin.AttrOn(goncurses.ColorPair(1))
		}

		displayEntries(detailWin, entries, maxX-listWidth-3, 0, state.showPasswords)

		if state.focusedPane == 1 {
			detailWin.AttrOff(goncurses.ColorPair(1))
		} else {
			groupWin.AttrOff(goncurses.ColorPair(1))
		}

		groupWin.Refresh()
		detailWin.Refresh()
		searchWin.Refresh()

		ch := stdscr.GetChar()

		switch ch {
		case 'e': // edit (placeholder)

		case 'f': // expand/collapse Group pane
			expandAllGroups = !expandAllGroups
			if expandAllGroups == false {
				state.selectedIndex = 0
				state.groupScrollPos = 0
			}
				expandCollapseGroups(rootGroup)
		case 'g': // go to top
			if state.focusedPane == 0 {
				state.selectedIndex = 0
				state.groupScrollPos = 0
			} else {
				state.entryScrollPos = 0
			}
		case 'G': // go to bottom
			if state.focusedPane == 0 {
				state.selectedIndex = len(visibleItems) - 1

				// Adjust group scroll to show the last item
				maxY, _ := groupWin.MaxYX()
				visibleLines := maxY - 2
				state.groupScrollPos = max(0, len(visibleItems)-visibleLines)
			} else {
				maxY, _ := detailWin.MaxYX()
				entriesPerPage := (maxY - 2) / 4
				state.entryScrollPos = max(0, len(entries)-entriesPerPage)
			}
		case 'q': // go to top
			return
		case ' ':
			state.showPasswords = !state.showPasswords
		case '/':
			state.inSearchMode = true
			expandAllGroups = true
			if state.inSearchMode {
				searchWin.MovePrint(1, 1, "/ ")
				searchWin.Refresh()
				goncurses.Echo(true)
				state.searchQuery = getString(searchWin, 1, 1, 30)
				goncurses.Echo(false)

				group, Index := searchGroups(rootGroup, state.searchQuery)
				if group != nil {
					currentGroup := group
					for currentGroup != nil {
						currentGroup.Expanded = true
						currentGroup = currentGroup.Parent
					}

					// Regenerate visible items
					visibleItems = getVisibleItems(rootGroup)

					// Calculate scroll position based on group number
					maxY, _ := groupWin.MaxYX()
					visibleLines := maxY - 2
					state.groupScrollPos = max(0, Index-visibleLines/2)

					// Find and select the group
					state.selectedIndex = group.Index + 1

				} else {
					state.searchQuery = "NOT FOUND"
				}
			}
			state.inSearchMode = false // exit search mode
		case goncurses.KEY_LEFT:
			state.focusedPane = 0
		case goncurses.KEY_RIGHT:
			state.focusedPane = 1
		case goncurses.KEY_UP:
			if state.focusedPane == 0 { // Groups
				if state.selectedIndex > 0 {
					state.selectedIndex--
					// Adjust scroll position if selected item would be out of view
					if state.selectedIndex < state.groupScrollPos {
						state.groupScrollPos = state.selectedIndex
					}
				}
			} else { // Entries
				if state.entryScrollPos > 0 {
					state.entryScrollPos--
				}
			}
		case goncurses.KEY_DOWN:
			if state.focusedPane == 0 { // Groups
				if state.selectedIndex < len(visibleItems)-1 {
					state.selectedIndex++
					// Calculate when to scroll
					maxY, _ := groupWin.MaxYX()
					visibleLines := maxY - 2 // Account for borders
					if state.selectedIndex >= state.groupScrollPos+visibleLines {
						state.groupScrollPos = state.selectedIndex - visibleLines + 1
					}
				}
			} else { // Entries
				maxY, _ := detailWin.MaxYX()
				entriesPerPage := (maxY - 2) / 4
				if len(entries) > entriesPerPage && state.entryScrollPos < len(entries)-entriesPerPage {
					state.entryScrollPos++
				}
			}

		case goncurses.KEY_ENTER, 10, 13:
			if state.focusedPane == 0 && state.selectedIndex >= 0 && state.selectedIndex < len(visibleItems) {
				selectedGroup := visibleItems[state.selectedIndex].Group
				if selectedGroup != nil {
					// log.Printf("Toggling group %s, current expanded state: %v", selectedGroup.Name, selectedGroup.Expanded)
					selectedGroup.Expanded = !selectedGroup.Expanded

					parent := selectedGroup.Parent
					for parent != nil {
						parent.Expanded = true
						parent = parent.Parent
					}

					state.entryScrollPos = 0

					// log.Printf("New expanded state for group %s: %v", selectedGroup.Name, selectedGroup.Expanded)
				}
			}
		}
	}
}

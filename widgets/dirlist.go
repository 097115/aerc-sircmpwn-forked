package widgets

import (
	"fmt"
	"log"
	"regexp"
	"sort"

	"github.com/gdamore/tcell"
	"github.com/mattn/go-runewidth"

	"git.sr.ht/~sircmpwn/aerc/config"
	"git.sr.ht/~sircmpwn/aerc/lib"
	"git.sr.ht/~sircmpwn/aerc/lib/ui"
	"git.sr.ht/~sircmpwn/aerc/models"
	"git.sr.ht/~sircmpwn/aerc/worker/types"
)

type DirectoryList struct {
	ui.Invalidatable
	aercConf  *config.AercConfig
	acctConf  *config.AccountConfig
	store     *lib.DirStore
	dirs      []string
	logger    *log.Logger
	selecting string
	selected  string
	spinner   *Spinner
	worker    *types.Worker
}

func NewDirectoryList(conf *config.AercConfig, acctConf *config.AccountConfig,
	logger *log.Logger, worker *types.Worker) *DirectoryList {

	dirlist := &DirectoryList{
		aercConf: conf,
		acctConf: acctConf,
		logger:   logger,
		store:    lib.NewDirStore(),
		worker:   worker,
	}
	uiConf := dirlist.UiConfig()
	dirlist.spinner = NewSpinner(&uiConf)
	dirlist.spinner.OnInvalidate(func(_ ui.Drawable) {
		dirlist.Invalidate()
	})
	dirlist.spinner.Start()
	return dirlist
}

func (dirlist *DirectoryList) UiConfig() config.UIConfig {
	return dirlist.aercConf.GetUiConfig(map[config.ContextType]string{
		config.UI_CONTEXT_ACCOUNT: dirlist.acctConf.Name,
		config.UI_CONTEXT_FOLDER:  dirlist.Selected(),
	})
}

func (dirlist *DirectoryList) List() []string {
	return dirlist.store.List()
}

func (dirlist *DirectoryList) UpdateList(done func(dirs []string)) {
	// TODO: move this logic into dirstore
	var dirs []string
	dirlist.worker.PostAction(
		&types.ListDirectories{}, func(msg types.WorkerMessage) {

			switch msg := msg.(type) {
			case *types.Directory:
				dirs = append(dirs, msg.Dir.Name)
			case *types.Done:
				dirlist.store.Update(dirs)
				dirlist.filterDirsByFoldersConfig()
				dirlist.sortDirsByFoldersSortConfig()
				dirlist.store.Update(dirlist.dirs)
				dirlist.spinner.Stop()
				dirlist.Invalidate()
				if done != nil {
					done(dirs)
				}
			}
		})
}

func (dirlist *DirectoryList) Select(name string) {
	dirlist.selecting = name
	dirlist.worker.PostAction(&types.OpenDirectory{Directory: name},
		func(msg types.WorkerMessage) {
			switch msg.(type) {
			case *types.Error:
				dirlist.selecting = ""
			case *types.Done:
				dirlist.selected = dirlist.selecting
				dirlist.filterDirsByFoldersConfig()
				hasSelected := false
				for _, d := range dirlist.dirs {
					if d == dirlist.selected {
						hasSelected = true
						break
					}
				}
				if !hasSelected && dirlist.selected != "" {
					dirlist.dirs = append(dirlist.dirs, dirlist.selected)
				}
				sort.Strings(dirlist.dirs)
				dirlist.sortDirsByFoldersSortConfig()
			}
			dirlist.Invalidate()
		})
	dirlist.Invalidate()
}

func (dirlist *DirectoryList) Selected() string {
	return dirlist.selected
}

func (dirlist *DirectoryList) Invalidate() {
	dirlist.DoInvalidate(dirlist)
}

func (dirlist *DirectoryList) getDirString(name string, width int, recentUnseen func() string) string {
	percent := false
	rightJustify := false
	formatted := ""
	doRightJustify := func(s string) {
		formatted = runewidth.FillRight(formatted, width-len(s))
		formatted = runewidth.Truncate(formatted, width-len(s), "…")
	}
	for _, char := range dirlist.UiConfig().DirListFormat {
		switch char {
		case '%':
			if percent {
				formatted += string(char)
				percent = false
			} else {
				percent = true
			}
		case '>':
			if percent {
				rightJustify = true
			}
		case 'n':
			if percent {
				if rightJustify {
					doRightJustify(name)
					rightJustify = false
				}
				formatted += name
				percent = false
			}
		case 'r':
			if percent {
				rString := recentUnseen()
				if rightJustify {
					doRightJustify(rString)
					rightJustify = false
				}
				formatted += rString
				percent = false
			}
		default:
			formatted += string(char)
		}
	}
	return formatted
}

func (dirlist *DirectoryList) getRUEString(name string) string {
	msgStore, ok := dirlist.MsgStore(name)
	if !ok {
		return ""
	}
	var totalRecent, totalUnseen, totalExists int
	if msgStore.DirInfo.AccurateCounts {
		totalRecent = msgStore.DirInfo.Recent
		totalUnseen = msgStore.DirInfo.Unseen
		totalExists = msgStore.DirInfo.Exists
	} else {
		totalRecent, totalUnseen, totalExists = countRUE(msgStore)
	}
	rueString := ""
	if totalRecent > 0 {
		rueString = fmt.Sprintf("%d/%d/%d", totalRecent, totalUnseen, totalExists)
	} else if totalUnseen > 0 {
		rueString = fmt.Sprintf("%d/%d", totalUnseen, totalExists)
	} else if totalExists > 0 {
		rueString = fmt.Sprintf("%d", totalExists)
	}
	return rueString
}

func (dirlist *DirectoryList) Draw(ctx *ui.Context) {
	ctx.Fill(0, 0, ctx.Width(), ctx.Height(), ' ', tcell.StyleDefault)

	if dirlist.spinner.IsRunning() {
		dirlist.spinner.Draw(ctx)
		return
	}

	if len(dirlist.dirs) == 0 {
		style := tcell.StyleDefault
		ctx.Printf(0, 0, style, dirlist.UiConfig().EmptyDirlist)
		return
	}

	row := 0
	for _, name := range dirlist.dirs {
		if row >= ctx.Height() {
			break
		}
		style := tcell.StyleDefault
		if name == dirlist.selected {
			style = style.Reverse(true)
		} else if name == dirlist.selecting {
			style = style.Reverse(true)
			style = style.Foreground(tcell.ColorGray)
		}
		ctx.Fill(0, row, ctx.Width(), 1, ' ', style)

		dirString := dirlist.getDirString(name, ctx.Width(), func() string {
			return dirlist.getRUEString(name)
		})

		ctx.Printf(0, row, style, dirString)
		row++
	}
}

func (dirlist *DirectoryList) MouseEvent(localX int, localY int, event tcell.Event) {
	switch event := event.(type) {
	case *tcell.EventMouse:
		switch event.Buttons() {
		case tcell.Button1:
			clickedDir, ok := dirlist.Clicked(localX, localY)
			if ok {
				dirlist.Select(clickedDir)
			}
		case tcell.WheelDown:
			dirlist.Next()
		case tcell.WheelUp:
			dirlist.Prev()
		}
	}
}

func (dirlist *DirectoryList) Clicked(x int, y int) (string, bool) {
	if dirlist.dirs == nil || len(dirlist.dirs) == 0 {
		return "", false
	}
	for i, name := range dirlist.dirs {
		if i == y {
			return name, true
		}
	}
	return "", false
}

func (dirlist *DirectoryList) NextPrev(delta int) {
	curIdx := findString(dirlist.dirs, dirlist.selected)
	if curIdx == len(dirlist.dirs) {
		return
	}
	newIdx := curIdx + delta
	ndirs := len(dirlist.dirs)

	if ndirs == 0 {
		return
	}

	if newIdx < 0 {
		newIdx = ndirs - 1
	} else if newIdx >= ndirs {
		newIdx = 0
	}

	dirlist.Select(dirlist.dirs[newIdx])
}

func (dirlist *DirectoryList) Next() {
	dirlist.NextPrev(1)
}

func (dirlist *DirectoryList) Prev() {
	dirlist.NextPrev(-1)
}

func folderMatches(folder string, pattern string) bool {
	if len(pattern) == 0 {
		return false
	}
	if pattern[0] == '~' {
		r, err := regexp.Compile(pattern[1:])
		if err != nil {
			return false
		}
		return r.Match([]byte(folder))
	}
	return pattern == folder
}

// sortDirsByFoldersSortConfig sets dirlist.dirs to be sorted based on the
// AccountConfig.FoldersSort option. Folders not included in the option
// will be appended at the end in alphabetical order
func (dirlist *DirectoryList) sortDirsByFoldersSortConfig() {
	sort.Slice(dirlist.dirs, func(i, j int) bool {
		foldersSort := dirlist.acctConf.FoldersSort
		iInFoldersSort := findString(foldersSort, dirlist.dirs[i])
		jInFoldersSort := findString(foldersSort, dirlist.dirs[j])
		if iInFoldersSort >= 0 && jInFoldersSort >= 0 {
			return iInFoldersSort < jInFoldersSort
		}
		if iInFoldersSort >= 0 {
			return true
		}
		if jInFoldersSort >= 0 {
			return false
		}
		return dirlist.dirs[i] < dirlist.dirs[j]
	})
}

// filterDirsByFoldersConfig sets dirlist.dirs to the filtered subset of the
// dirstore, based on the AccountConfig.Folders option
func (dirlist *DirectoryList) filterDirsByFoldersConfig() {
	dirlist.dirs = dirlist.store.List()
	// config option defaults to show all if unset
	configFolders := dirlist.acctConf.Folders
	if len(configFolders) == 0 {
		return
	}
	var filtered []string
	for _, folder := range dirlist.dirs {
		for _, cfgfolder := range configFolders {
			if folderMatches(folder, cfgfolder) {
				filtered = append(filtered, folder)
				break
			}
		}
	}
	dirlist.dirs = filtered
}

func (dirlist *DirectoryList) SelectedMsgStore() (*lib.MessageStore, bool) {
	return dirlist.store.MessageStore(dirlist.selected)
}

func (dirlist *DirectoryList) MsgStore(name string) (*lib.MessageStore, bool) {
	return dirlist.store.MessageStore(name)
}

func (dirlist *DirectoryList) SetMsgStore(name string, msgStore *lib.MessageStore) {
	dirlist.store.SetMessageStore(name, msgStore)
	msgStore.OnUpdateDirs(func() {
		dirlist.Invalidate()
	})
}

func findString(slice []string, str string) int {
	for i, s := range slice {
		if str == s {
			return i
		}
	}
	return -1
}

func (dirlist *DirectoryList) getSortCriteria() []*types.SortCriterion {
	if len(dirlist.UiConfig().Sort) == 0 {
		return nil
	}
	criteria, err := libsort.GetSortCriteria(dirlist.UiConfig().Sort)
	if err != nil {
		dirlist.logger.Printf("getSortCriteria failed: %v", err)
		return nil
	}
	return criteria
}

func countRUE(msgStore *lib.MessageStore) (recent, unread, exist int) {
	for _, msg := range msgStore.Messages {
		if msg == nil {
			continue
		}
		seen := false
		isrecent := false
		for _, flag := range msg.Flags {
			if flag == models.SeenFlag {
				seen = true
			} else if flag == models.RecentFlag {
				isrecent = true
			}
		}
		if !seen {
			if isrecent {
				recent++
			} else {
				unread++
			}
		}
		exist++
	}
	return recent, unread, exist
}

package tracker

import "testing"

func TestPageTitle(t *testing.T) {
	cases := map[string]string{
		"History Plus and 8 more pages - Personal - Microsoft\u200b Edge": "History Plus",
		"GitHub - Google Chrome": "GitHub",
		"Trending repositories on GitHub today · GitHub - Mozilla Firefox": "Trending repositories on GitHub today · GitHub",
		"上网习惯 和另外 3 个页面 - 个人 - Microsoft Edge":                             "上网习惯",
		"Inbox (3) - user@example.com - Gmail - Google Chrome":             "Inbox (3) - user@example.com - Gmail",
		// Profiles: after the name (Chrome/Edge on a Mac), before it (Edge on Windows).
		"Example Domain - Google Chrome - Jonathan":                     "Example Domain",
		"Inbox - user@example.com - Gmail - Google Chrome - Work":       "Inbox - user@example.com - Gmail",
		"Example Domain - Google Chrome Beta - Profile 2":               "Example Domain",
		"Example Domain - Microsoft\u200b Edge - Personal":              "Example Domain",
		"GitHub - Personal - Microsoft\u200b Edge":                      "GitHub",
		"Inbox - user@example.com - Gmail - Profile 1 - Microsoft Edge": "Inbox - user@example.com - Gmail",
		"新标签页 - 个人 - Microsoft Edge":                                    "新标签页",
		// Titles that only mention a browser keep their last part (Safari adds no suffix).
		"Carmen - Opera - Wikipedia": "Carmen - Opera - Wikipedia",
		"Brave - Search":             "Brave - Search",
	}
	for in, want := range cases {
		if got := PageTitle(in); got != want {
			t.Errorf("PageTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFileFromTitle(t *testing.T) {
	cases := map[string]string{
		"Report Q3.docx - Word":                       "Report Q3.docx",
		"● main.go - timestamps - Visual Studio Code": "main.go",
		"询价单-扩展测试数据.xlsx - WPS Office":                "询价单-扩展测试数据.xlsx",
		`C:\Users\me\notes.txt - Notepad++`:           `C:\Users\me\notes.txt`,
		"*untitled.txt - Notepad":                     "untitled.txt",
		"WeChat":                                      "",
		"github.com - Discord":                        "",
		"Budget.xlsx [Read-Only] - Excel":             "Budget.xlsx",
	}
	for in, want := range cases {
		if got := fileFromTitle(in); got != want {
			t.Errorf("fileFromTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWebDoc(t *testing.T) {
	if d := webDoc("extension://abc/static/app.html", "x", false); d.Kind != 0 {
		t.Errorf("extension page should be ignored, got %+v", d)
	}
	if d := webDoc("github.com/trending", "Trending", false); d.Key != "https://github.com/trending" || d.Grp != "github.com" {
		t.Errorf("bare host not normalised: %+v", d)
	}
	if d := webDoc("search terms typed", "x", false); d.Kind != 0 {
		t.Errorf("typed search text should be ignored, got %+v", d)
	}
	if d := webDoc("https://www.bilibili.com/video/BV1#t=3", "Video", false); d.Grp != "bilibili.com" || d.Key != "https://www.bilibili.com/video/BV1" {
		t.Errorf("unexpected %+v", d)
	}
}

package i18n

// messagesKO is the Korean catalog. Keys must match messagesEN; T falls
// back to English for anything missing here rather than erroring, so a
// partial translation still works (just mixes languages for the gaps).
var messagesKO = map[string]string{
	// ---- Tray menu (tray/tray.go) ----
	"tray.indefinite.label":     "무기한",
	"tray.indefinite.tooltip":   "시스템을 무기한으로 깨어있게 유지",
	"tray.preset.label":         "프리셋",
	"tray.preset.tooltip":       "프리셋 시간 선택",
	"tray.preset.start_tooltip": "%s 시작",
	"tray.custom.label":         "사용자 지정...",
	"tray.custom.tooltip":       "지정한 시간 동안 또는 특정 시각까지 시작",
	"tray.turnoff.label":        "끄기",
	"tray.turnoff.tooltip":      "타이머 끄기",
	"tray.keepdisplay.label":    "화면 켜짐 유지",
	"tray.keepdisplay.tooltip":  "화면도 꺼지지 않도록 유지",
	"tray.settings.label":       "설정",
	"tray.settings.tooltip":     "설정 창 열기",
	"tray.about.label":          "정보",
	"tray.about.tooltip":        "mugcup 정보",
	"tray.quit.label":           "종료",
	"tray.quit.tooltip":         "mugcup 종료",

	// ---- Popup window titles/dialogs (wailsapp.go) ----
	"window.title.settings":      "설정",
	"window.title.custom":        "사용자 지정",
	"window.title.about":         "정보",
	"dialog.export_config.title": "mugcup 설정 내보내기",
	"dialog.import_config.title": "mugcup 설정 가져오기",
	"config.export.success":      "설정을 다음 위치로 내보냈습니다:\n%s",
	"config.import.success":      "설정을 가져왔습니다.",

	// ---- Native message boxes (main.go, updateflow.go, webview2.go) ----
	"cli_warning.text": "mugcup.exe는 명령줄 인수를 직접 처리하지 않습니다.\n\n" +
		"실행 중인 mugcup에 명령을 보내려면 mugcup-cli.exe를 사용하세요.\n예: mugcup-cli start 1h30m",
	"update.available.title": "mugcup 업데이트 사용 가능",
	"update.available.text":  "mugcup %s를 사용할 수 있습니다 (현재 버전: %s).\n\n지금 설치할까요? mugcup이 다시 시작됩니다.",
	"update.failed.title":    "mugcup 업데이트 실패",
	"update.failed.prefix":   "mugcup 업데이트에 실패했습니다:\n\n",
	"update.up_to_date":      "mugcup %s는 이미 최신 버전입니다.",
	"update.dev_build":       "개발 빌드에서는 업데이트를 확인할 수 없습니다.",
	"update.restart_needed":  "mugcup이 %s로 업데이트되었습니다. 완료하려면 다시 시작해주세요.",
	"webview2.missing": "이 창을 표시하려면 Microsoft Edge WebView2 런타임이 필요하지만, 이 PC에는 설치되어 있지 않습니다.\n\n" +
		"https://developer.microsoft.com/microsoft-edge/webview2/ 에서 설치한 뒤 다시 시도해주세요. " +
		"트레이의 화면 유지 기능은 이 런타임 없이도 정상 작동합니다.",

	// ---- Frontend UI (frontend/dist/index.html, app.js) ----
	"ui.settings.presets_heading":       "프리셋",
	"ui.settings.preset_placeholder":    "예: 1h30m, 45m, 무기한은 0",
	"ui.settings.add_button":            "+ 추가",
	"ui.settings.general_heading":       "일반",
	"ui.settings.keep_display_on":       "화면 켜짐 유지",
	"ui.settings.start_with_windows":    "Windows 시작 시 자동 실행",
	"ui.settings.auto_check_updates":    "업데이트 자동 확인",
	"ui.settings.auto_install_updates":  "업데이트 자동 설치",
	"ui.settings.tray_click_action":     "트레이 클릭 동작",
	"ui.settings.tray_click_cycle":      "프리셋 순환",
	"ui.settings.tray_click_indefinite": "무기한 모드 토글",
	"ui.settings.tray_click_menu":       "트레이 메뉴 열기",
	"ui.settings.config_heading":        "설정 파일",
	"ui.settings.export_button":         "내보내기...",
	"ui.settings.import_button":         "가져오기...",
	"ui.settings.language":              "언어",

	"ui.custom.heading":              "사용자 지정",
	"ui.custom.tab_duration":         "지정 시간 동안",
	"ui.custom.tab_until":            "특정 날짜·시각까지",
	"ui.custom.duration_placeholder": "예: 1h30m, 45m",
	"ui.custom.start_button":         "시작",
	"ui.custom.choose_date":          "특정 날짜 선택",
	"ui.custom.cancel_button":        "취소",

	"ui.about.tagline":        "당신의 방식대로, PC를 깨어있게.",
	"ui.about.version_prefix": "버전",
	"ui.about.view_github":    "GitHub에서 보기",
	"ui.about.check_updates":  "업데이트 확인",
	"ui.about.close":          "닫기",

	"ui.common.indefinite":   "무기한",
	"ui.common.off":          "꺼짐",
	"ui.common.time_left_hm": "%d시간 %d분 남음",
	"ui.common.time_left_m":  "%d분 남음",
	"ui.common.until":        "%s까지",

	"ui.preset.empty":        "아직 프리셋이 없습니다 — 아래에서 추가하세요.",
	"ui.preset.drag_title":   "드래그하여 순서 변경",
	"ui.preset.remove_title": "프리셋 삭제",

	"ui.error.unrecognized_with_indefinite": "인식할 수 없는 형식입니다. 예: 1h30m, 45m, 무기한은 0.",
	"ui.error.preset_exists":                "이미 존재하는 프리셋입니다.",
	"ui.error.enter_duration":               "시간을 입력하세요. 예: 1h30m, 45m.",
	"ui.error.unrecognized_duration":        "인식할 수 없는 형식입니다. 예: 1h30m, 45m.",
	"ui.error.pick_datetime":                "날짜와 시각을 선택하세요.",
	"ui.error.pick_time":                    "시각을 선택하세요.",
	"ui.error.pick_future_datetime":         "미래의 날짜와 시각을 선택하세요.",
	"ui.error.pick_future_time":             `오늘의 이후 시각을 선택하거나, "날짜 선택"을 켜서 다른 날을 선택하세요.`,
}

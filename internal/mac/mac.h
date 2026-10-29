// Declarations shared by the Go and Objective-C halves of package mac.
// Strings handed to Go are malloc'd; the Go side frees them.
#ifndef TIMESTAMPS_MAC_H
#define TIMESTAMPS_MAC_H

#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

// The frontmost app and its focused window.
typedef struct {
	int pid;
	char *name;     // localized app name
	char *bundle;   // the .app folder, or the executable of a plain process
	char *bundleID;
	char *title;    // window title, NULL when unknown
	uint64_t window; // identity of the focused window
	double x, y, w, h; // its frame in global screen points; w == 0: unknown
} TSFront;

// Tracking (sys_darwin.m).
bool tsFront(TSFront *f);
void tsFreeFront(TSFront *f);
char *tsFocusedDocument(int pid);
char *tsBundleID(int pid);
char *tsBrowserURL(int pid, int dialect, bool *incognito);
void *tsAppIconPNG(const char *path, int size, int *len);
double tsIdleSeconds(void);
bool tsScreenLocked(void);
bool tsSoundPlaying(void);
bool tsDisplayAwake(void);
bool tsAccessibilityTrusted(bool prompt);
bool tsScreenCaptureAllowed(void);
void tsRequestScreenCapture(void);
void tsOpenPrivacyPane(const char *pane);
char *tsSpotlightPath(const char *name);
void *tsCapture(double x, double y, bool atPoint, bool all, int *w, int *h);

// The app (shell_darwin.m).
void tsAppPrepare(void);
void tsAppInit(const char *labels);
void tsAppRun(void);
void tsSetTip(const char *tip);
void tsNotify(const char *title, const char *text, const char *route);
void tsShowWindow(const char *url, const char *script, const char *background);
void tsSetAppearance(int mode);
void tsSetStatusIcon(int points, const void *png1, int len1, const void *png2, int len2);
void tsSetAppIcon(const void *png, int len);
bool tsSystemDark(void);
void tsQuit(void);
char *tsPickFile(const char *title, const char *exts, const char *dir);
char *tsPickFolder(const char *title, const char *dir);
void tsOpen(const char *target);
void tsReveal(const char *path);
void tsAlert(const char *title, const char *text);
bool tsAsk(const char *title, const char *text, const char *ok, const char *cancel);
char *tsPreferredLanguage(void);

#endif

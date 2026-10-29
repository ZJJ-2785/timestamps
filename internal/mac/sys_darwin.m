// Tracking on macOS: the frontmost app and its window, open documents,
// browser tabs, idle time, the lock screen, playing sound, app icons,
// Spotlight lookups and screen capture.
#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreAudio/CoreAudio.h>
#import <CoreServices/CoreServices.h>
#import <ScriptingBridge/ScriptingBridge.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#include <dlfcn.h>
#include "mac.h"

#define K(x) ((__bridge NSString *)(x))

static char *dupstr(NSString *s) {
	if (s == nil) return NULL;
	const char *u = s.UTF8String;
	return u ? strdup(u) : NULL;
}

// ---- frontmost app and window ---------------------------------------------

static void axInit(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		// The global timeout: an app that doesn't answer can't stall tracking.
		AXUIElementRef sys = AXUIElementCreateSystemWide();
		AXUIElementSetMessagingTimeout(sys, 0.5f);
		CFRelease(sys);
	});
}

static CFTypeRef axCopy(AXUIElementRef el, CFStringRef attr) {
	CFTypeRef v = NULL;
	if (AXUIElementCopyAttributeValue(el, attr, &v) != kAXErrorSuccess) return NULL;
	return v;
}

static AXUIElementRef focusedWindow(pid_t pid) {
	axInit();
	AXUIElementRef app = AXUIElementCreateApplication(pid);
	if (app == NULL) return NULL;
	CFTypeRef w = axCopy(app, kAXFocusedWindowAttribute);
	CFRelease(app);
	if (w != NULL && CFGetTypeID(w) != AXUIElementGetTypeID()) {
		CFRelease(w);
		return NULL;
	}
	return (AXUIElementRef)w;
}

// windowListInfo fills in what the Accessibility API didn't give from the
// window list: the frame always, the title only with Screen Recording.
static void windowListInfo(pid_t pid, TSFront *f) {
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (list == NULL) return;
	for (NSDictionary *w in (__bridge NSArray *)list) {
		if ([w[K(kCGWindowOwnerPID)] intValue] != pid || [w[K(kCGWindowLayer)] intValue] != 0) continue;
		NSString *name = w[K(kCGWindowName)];
		if (f->title == NULL && [name isKindOfClass:NSString.class] && name.length > 0) f->title = dupstr(name);
		CGRect r;
		NSDictionary *b = w[K(kCGWindowBounds)];
		if (f->w == 0 && b != nil && CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)b, &r)) {
			f->x = r.origin.x;
			f->y = r.origin.y;
			f->w = r.size.width;
			f->h = r.size.height;
		}
		if ((f->window & 0xffffffffu) == 0) f->window |= [w[K(kCGWindowNumber)] unsignedIntValue];
		break;
	}
	CFRelease(list);
}

bool tsFront(TSFront *f) {
	@autoreleasepool {
		memset(f, 0, sizeof *f);
		NSRunningApplication *app = NSWorkspace.sharedWorkspace.frontmostApplication;
		if (app == nil) return false;
		pid_t pid = app.processIdentifier;
		NSURL *url = app.bundleURL ?: app.executableURL;
		NSString *name = app.localizedName;
		if (name.length == 0) name = url.lastPathComponent.stringByDeletingPathExtension;
		f->pid = pid;
		f->name = dupstr(name);
		f->bundle = dupstr(url.path);
		f->bundleID = dupstr(app.bundleIdentifier);
		f->window = (uint64_t)pid << 32;
		if (AXIsProcessTrusted()) {
			AXUIElementRef w = focusedWindow(pid);
			if (w != NULL) {
				CFTypeRef t = axCopy(w, kAXTitleAttribute);
				if (t != NULL) {
					if (CFGetTypeID(t) == CFStringGetTypeID()) f->title = dupstr((__bridge NSString *)t);
					CFRelease(t);
				}
				CFTypeRef pv = axCopy(w, kAXPositionAttribute), sv = axCopy(w, kAXSizeAttribute);
				CGPoint p;
				CGSize s;
				if (pv != NULL && sv != NULL && CFGetTypeID(pv) == AXValueGetTypeID() && CFGetTypeID(sv) == AXValueGetTypeID() &&
					AXValueGetValue((AXValueRef)pv, kAXValueTypeCGPoint, &p) && AXValueGetValue((AXValueRef)sv, kAXValueTypeCGSize, &s)) {
					f->x = p.x;
					f->y = p.y;
					f->w = s.width;
					f->h = s.height;
				}
				if (pv != NULL) CFRelease(pv);
				if (sv != NULL) CFRelease(sv);
				f->window |= (uint32_t)CFHash(w);
				CFRelease(w);
			}
		}
		if (f->title == NULL || f->w == 0) windowListInfo(pid, f);
		return true;
	}
}

void tsFreeFront(TSFront *f) {
	free(f->name);
	free(f->bundle);
	free(f->bundleID);
	free(f->title);
	memset(f, 0, sizeof *f);
}

char *tsBundleID(int pid) {
	@autoreleasepool {
		return dupstr([NSRunningApplication runningApplicationWithProcessIdentifier:pid].bundleIdentifier);
	}
}

// ---- documents -------------------------------------------------------------

// tsFocusedDocument returns the file shown by the focused window of an app
// (AXDocument: what document-based apps put in the title bar proxy icon).
char *tsFocusedDocument(int pid) {
	@autoreleasepool {
		if (!AXIsProcessTrusted()) return NULL;
		AXUIElementRef w = focusedWindow(pid);
		if (w == NULL) return NULL;
		char *res = NULL;
		CFTypeRef d = axCopy(w, kAXDocumentAttribute);
		if (d != NULL) {
			NSURL *u = nil;
			if (CFGetTypeID(d) == CFStringGetTypeID()) {
				NSString *s = (__bridge NSString *)d;
				u = [s hasPrefix:@"/"] ? [NSURL fileURLWithPath:s] : [NSURL URLWithString:s];
			} else if (CFGetTypeID(d) == CFURLGetTypeID()) {
				u = (__bridge NSURL *)d;
			}
			if (u.isFileURL && u.path.length > 0) res = dupstr(u.path);
			CFRelease(d);
		}
		CFRelease(w);
		return res;
	}
}

// tsSpotlightPath finds the most recently used file with this name.
char *tsSpotlightPath(const char *name) {
	@autoreleasepool {
		NSString *n = [NSString stringWithUTF8String:name];
		if (n.length == 0) return NULL;
		for (NSString *c in @[@"\\", @"\"", @"*", @"?"]) n = [n stringByReplacingOccurrencesOfString:c withString:[@"\\" stringByAppendingString:c]];
		NSString *q = [NSString stringWithFormat:@"kMDItemFSName == \"%@\"", n];
		MDQueryRef query = MDQueryCreate(kCFAllocatorDefault, (__bridge CFStringRef)q, NULL, NULL);
		if (query == NULL) return NULL;
		MDQuerySetMaxCount(query, 50);
		NSString *best = nil;
		NSDate *bestUsed = nil;
		if (MDQueryExecute(query, kMDQuerySynchronous)) {
			CFIndex count = MDQueryGetResultCount(query);
			for (CFIndex i = 0; i < count; i++) {
				MDItemRef item = (MDItemRef)MDQueryGetResultAtIndex(query, i);
				NSString *path = CFBridgingRelease(MDItemCopyAttribute(item, kMDItemPath));
				if (![path isKindOfClass:NSString.class]) continue;
				NSDate *used = CFBridgingRelease(MDItemCopyAttribute(item, kMDItemLastUsedDate));
				if (![used isKindOfClass:NSDate.class]) used = CFBridgingRelease(MDItemCopyAttribute(item, kMDItemContentModificationDate));
				if (![used isKindOfClass:NSDate.class]) used = nil;
				if (best == nil || (used != nil && (bestUsed == nil || [used compare:bestUsed] == NSOrderedDescending))) {
					best = path;
					bestUsed = used;
				}
			}
		}
		CFRelease(query);
		return dupstr(best);
	}
}

// ---- browser tabs ----------------------------------------------------------

@interface TSQuietScripting : NSObject <SBApplicationDelegate>
@end

@implementation TSQuietScripting
- (id)eventDidFail:(const AppleEvent *)event withError:(NSError *)error {
	return nil;
}
@end

static NSMutableDictionary<NSNumber *, SBApplication *> *sbApps;
static NSMutableSet<NSString *> *aeAsked;
static TSQuietScripting *sbQuiet;

static void sbInit(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		sbApps = [NSMutableDictionary new];
		aeAsked = [NSMutableSet new];
		sbQuiet = [TSQuietScripting new];
	});
}

// mayAutomate reports whether the user allowed Timestamps to ask this app
// for its tabs. The first time, macOS asks the user on another thread, so
// the tracker never waits for the answer.
static bool mayAutomate(NSString *bundleID) {
	NSAppleEventDescriptor *target = [NSAppleEventDescriptor descriptorWithBundleIdentifier:bundleID];
	OSStatus st = AEDeterminePermissionToAutomateTarget(target.aeDesc, typeWildCard, typeWildCard, false);
	if (st == -1744) { // errAEEventWouldRequireUserConsent
		@synchronized (aeAsked) {
			if (![aeAsked containsObject:bundleID]) {
				[aeAsked addObject:bundleID];
				dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
					AEDeterminePermissionToAutomateTarget(target.aeDesc, typeWildCard, typeWildCard, true);
				});
			}
		}
	}
	return st == noErr;
}

// tsBrowserURL asks a browser for the address of the tab in its front
// window over Apple events: dialect 1 is Chromium's scripting dictionary
// (Chrome, Edge, Brave, Vivaldi, Arc, Opera), 2 is Safari's.
char *tsBrowserURL(int pid, int dialect, bool *incognito) {
	@autoreleasepool {
		*incognito = false;
		sbInit();
		NSString *bid = [NSRunningApplication runningApplicationWithProcessIdentifier:pid].bundleIdentifier;
		if (bid == nil || !mayAutomate(bid)) return NULL;
		@try {
			SBApplication *app;
			@synchronized (sbApps) {
				app = sbApps[@(pid)];
				if (app == nil) {
					app = [SBApplication applicationWithProcessIdentifier:pid];
					if (app == nil) return NULL;
					app.delegate = sbQuiet;
					app.timeout = 45; // ticks of 1/60 s
					if (sbApps.count > 16) [sbApps removeAllObjects];
					sbApps[@(pid)] = app;
				}
			}
			SBElementArray *wins = [app valueForKey:@"windows"];
			if (wins.count == 0) return NULL;
			id w = [wins objectAtIndex:0];
			id tab = [w valueForKey:(dialect == 2 ? @"currentTab" : @"activeTab")];
			id url = [tab valueForKey:@"URL"];
			if (dialect == 1) {
				id mode = [w valueForKey:@"mode"];
				*incognito = [mode isKindOfClass:NSString.class] && [mode isEqualToString:@"incognito"];
			}
			return [url isKindOfClass:NSString.class] ? dupstr(url) : NULL;
		} @catch (NSException *e) {
			return NULL;
		}
	}
}

// ---- idle, lock screen, sound ---------------------------------------------

double tsIdleSeconds(void) {
	return CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateCombinedSessionState, kCGAnyInputEventType);
}

static bool truthy(CFTypeRef v) {
	if (v == NULL) return false;
	if (CFGetTypeID(v) == CFBooleanGetTypeID()) return CFBooleanGetValue((CFBooleanRef)v);
	if (CFGetTypeID(v) == CFNumberGetTypeID()) {
		int n = 0;
		CFNumberGetValue((CFNumberRef)v, kCFNumberIntType, &n);
		return n != 0;
	}
	return false;
}

// tsScreenLocked reports a locked screen, or a session switched away from.
bool tsScreenLocked(void) {
	CFDictionaryRef d = CGSessionCopyCurrentDictionary();
	if (d == NULL) return false;
	bool locked = truthy(CFDictionaryGetValue(d, CFSTR("CGSSessionScreenIsLocked")));
	CFTypeRef on = CFDictionaryGetValue(d, kCGSessionOnConsoleKey);
	if (on != NULL && !truthy(on)) locked = true;
	CFRelease(d);
	return locked;
}

// tsDisplayAwake reports whether any display is on. While the lid is
// closed, Power Nap runs the app with every display asleep.
bool tsDisplayAwake(void) {
	CGDirectDisplayID ids[16];
	uint32_t n = 0;
	if (CGGetOnlineDisplayList(16, ids, &n) != kCGErrorSuccess) return true;
	for (uint32_t i = 0; i < n; i++) {
		if (!CGDisplayIsAsleep(ids[i])) return true;
	}
	return false;
}

// tsSoundPlaying reports whether any app is playing through the default
// output device.
bool tsSoundPlaying(void) {
	AudioObjectPropertyAddress a = {kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal, 0};
	AudioDeviceID dev = 0;
	UInt32 size = sizeof dev;
	if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, &dev) != noErr || dev == 0) return false;
	a.mSelector = kAudioDevicePropertyDeviceIsRunningSomewhere;
	UInt32 running = 0;
	size = sizeof running;
	if (AudioObjectGetPropertyData(dev, &a, 0, NULL, &size, &running) != noErr) return false;
	return running != 0;
}

// ---- permissions -----------------------------------------------------------

bool tsAccessibilityTrusted(bool prompt) {
	@autoreleasepool {
		if (!prompt) return AXIsProcessTrusted();
		NSDictionary *opts = @{K(kAXTrustedCheckOptionPrompt): @YES};
		return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)opts);
	}
}

bool tsScreenCaptureAllowed(void) { return CGPreflightScreenCaptureAccess(); }

void tsRequestScreenCapture(void) { CGRequestScreenCaptureAccess(); }

void tsOpenPrivacyPane(const char *pane) {
	@autoreleasepool {
		NSString *u = [@"x-apple.systempreferences:com.apple.preference.security?" stringByAppendingString:[NSString stringWithUTF8String:pane]];
		[NSWorkspace.sharedWorkspace openURL:[NSURL URLWithString:u]];
	}
}

// ---- icons -----------------------------------------------------------------

// tsAppIconPNG renders the Finder icon of an app as a size x size PNG,
// without most of the empty margin macOS icons have.
void *tsAppIconPNG(const char *path, int size, int *len) {
	@autoreleasepool {
		*len = 0;
		NSImage *img = [NSWorkspace.sharedWorkspace iconForFile:[NSString stringWithUTF8String:path]];
		if (img == nil) return NULL;
		NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:size pixelsHigh:size bitsPerSample:8
			samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
		if (rep == nil) return NULL;
		rep.size = NSMakeSize(size, size);
		NSGraphicsContext *ctx = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
		if (ctx == nil) return NULL;
		[NSGraphicsContext saveGraphicsState];
		NSGraphicsContext.currentContext = ctx;
		ctx.imageInterpolation = NSImageInterpolationHigh;
		NSSize is = img.size;
		NSRect from = NSInsetRect(NSMakeRect(0, 0, is.width, is.height), is.width * 0.06, is.height * 0.06);
		[img drawInRect:NSMakeRect(0, 0, size, size) fromRect:from operation:NSCompositingOperationSourceOver fraction:1.0];
		[ctx flushGraphics];
		[NSGraphicsContext restoreGraphicsState];
		NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		if (png.length == 0) return NULL;
		void *out = malloc(png.length);
		memcpy(out, png.bytes, png.length);
		*len = (int)png.length;
		return out;
	}
}

// ---- screen capture --------------------------------------------------------

API_AVAILABLE(macos(14.0))
static CGImageRef captureWithScreenCaptureKit(CGDirectDisplayID did) {
	// ScreenCaptureKit is loaded at run time so the app still starts on
	// macOS versions without it.
	static dispatch_once_t once;
	static bool loaded;
	dispatch_once(&once, ^{
		loaded = dlopen("/System/Library/Frameworks/ScreenCaptureKit.framework/ScreenCaptureKit", RTLD_LAZY) != NULL;
	});
	Class content = NSClassFromString(@"SCShareableContent");
	if (!loaded || content == nil) return NULL;
	__block CGImageRef result = NULL;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[content getShareableContentExcludingDesktopWindows:NO onScreenWindowsOnly:YES completionHandler:^(SCShareableContent *sc, NSError *err) {
		SCDisplay *display = nil;
		for (SCDisplay *d in sc.displays) {
			if (d.displayID == did) display = d;
		}
		if (display == nil) {
			dispatch_semaphore_signal(done);
			return;
		}
		SCContentFilter *filter = [[NSClassFromString(@"SCContentFilter") alloc] initWithDisplay:display excludingWindows:@[]];
		SCStreamConfiguration *cfg = [[NSClassFromString(@"SCStreamConfiguration") alloc] init];
		cfg.width = (size_t)(filter.contentRect.size.width * filter.pointPixelScale);
		cfg.height = (size_t)(filter.contentRect.size.height * filter.pointPixelScale);
		cfg.showsCursor = NO;
		[NSClassFromString(@"SCScreenshotManager") captureImageWithFilter:filter configuration:cfg completionHandler:^(CGImageRef img, NSError *e) {
			if (img != NULL) result = CGImageRetain(img);
			dispatch_semaphore_signal(done);
		}];
	}];
	if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC)) != 0) return NULL;
	return result;
}

static CGImageRef captureDisplay(CGDirectDisplayID did) {
	if (@available(macOS 14.0, *)) return captureWithScreenCaptureKit(did);
	// Before ScreenCaptureKit's screenshots: the CoreGraphics call, looked
	// up at run time because newer SDKs no longer offer it.
	typedef CGImageRef (*createImageFn)(CGDirectDisplayID);
	static createImageFn createImage;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		createImage = (createImageFn)dlsym(RTLD_DEFAULT, "CGDisplayCreateImage");
	});
	return createImage != NULL ? createImage(did) : NULL;
}

static CGContextRef rgbaContext(size_t w, size_t h) {
	CGColorSpaceRef cs = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	CGContextRef ctx = CGBitmapContextCreate(NULL, w, h, 8, w * 4, cs, (CGBitmapInfo)kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
	CGColorSpaceRelease(cs);
	return ctx;
}

static void *contextPixels(CGContextRef ctx, int *w, int *h) {
	size_t cw = CGBitmapContextGetWidth(ctx), ch = CGBitmapContextGetHeight(ctx), stride = CGBitmapContextGetBytesPerRow(ctx);
	const uint8_t *src = CGBitmapContextGetData(ctx);
	if (src == NULL) return NULL;
	uint8_t *out = malloc(cw * ch * 4);
	for (size_t y = 0; y < ch; y++) memcpy(out + y * cw * 4, src + y * stride, cw * 4);
	*w = (int)cw;
	*h = (int)ch;
	return out;
}

// tsCapture returns the RGBA pixels of the display containing (x, y) (or
// the main display), or of all displays side by side as they are arranged.
void *tsCapture(double x, double y, bool atPoint, bool all, int *w, int *h) {
	@autoreleasepool {
		*w = *h = 0;
		CGDirectDisplayID ids[16];
		uint32_t n = 0;
		if (all) {
			if (CGGetActiveDisplayList(16, ids, &n) != kCGErrorSuccess) n = 0;
		} else if (atPoint) {
			if (CGGetDisplaysWithPoint(CGPointMake(x, y), 1, ids, &n) != kCGErrorSuccess) n = 0;
		}
		if (n == 0) {
			ids[0] = CGMainDisplayID();
			n = 1;
		}
		if (n == 1) {
			CGImageRef img = captureDisplay(ids[0]);
			if (img == NULL) return NULL;
			size_t iw = CGImageGetWidth(img), ih = CGImageGetHeight(img);
			CGContextRef ctx = rgbaContext(iw, ih);
			void *px = NULL;
			if (ctx != NULL) {
				CGContextDrawImage(ctx, CGRectMake(0, 0, iw, ih), img);
				px = contextPixels(ctx, w, h);
				CGContextRelease(ctx);
			}
			CGImageRelease(img);
			return px;
		}
		// Several displays: one picture in points, each display at its place.
		CGRect bounds = CGRectNull;
		for (uint32_t i = 0; i < n; i++) bounds = CGRectUnion(bounds, CGDisplayBounds(ids[i]));
		CGContextRef ctx = rgbaContext((size_t)bounds.size.width, (size_t)bounds.size.height);
		if (ctx == NULL) return NULL;
		CGContextSetRGBFillColor(ctx, 0, 0, 0, 1);
		CGContextFillRect(ctx, CGRectMake(0, 0, bounds.size.width, bounds.size.height));
		CGContextSetInterpolationQuality(ctx, kCGInterpolationHigh);
		bool any = false;
		for (uint32_t i = 0; i < n; i++) {
			CGImageRef img = captureDisplay(ids[i]);
			if (img == NULL) continue;
			CGRect b = CGDisplayBounds(ids[i]);
			CGRect r = CGRectMake(b.origin.x - bounds.origin.x, bounds.size.height - (b.origin.y - bounds.origin.y) - b.size.height, b.size.width, b.size.height);
			CGContextDrawImage(ctx, r, img);
			CGImageRelease(img);
			any = true;
		}
		void *px = any ? contextPixels(ctx, w, h) : NULL;
		CGContextRelease(ctx);
		return px;
	}
}

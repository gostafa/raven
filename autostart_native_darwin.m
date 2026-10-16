//go:build darwin && !ios && !server

#import <Cocoa/Cocoa.h>
#import <CoreServices/CoreServices.h>

extern void ravenLoginLaunchDetected(int automatic);

// Capture the Apple event synchronously, while Cocoa is still handling launch.
// Wails forwards its application events to Go asynchronously, after this context
// may have gone away. SMAppService.mainAppService doesn't pass custom arguments.
@interface RavenLoginLaunchObserver : NSObject
- (void)didFinishLaunching:(NSNotification *)notification;
@end

@implementation RavenLoginLaunchObserver
- (void)didFinishLaunching:(NSNotification *)notification {
    NSAppleEventDescriptor *event = NSAppleEventManager.sharedAppleEventManager.currentAppleEvent;
    BOOL automatic = event.eventID == kAEOpenApplication &&
        [[event paramDescriptorForKeyword:keyAEPropData] enumCodeValue] == keyAELaunchedAsLogInItem;
    ravenLoginLaunchDetected(automatic ? 1 : 0);
    [[NSNotificationCenter defaultCenter] removeObserver:self];
}
@end

void ravenPrepareLoginDetection(void) {
    static RavenLoginLaunchObserver *observer;
    observer = [RavenLoginLaunchObserver new];
    [[NSNotificationCenter defaultCenter] addObserver:observer
        selector:@selector(didFinishLaunching:)
        name:NSApplicationDidFinishLaunchingNotification object:nil];
}

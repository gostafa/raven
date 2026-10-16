//go:build darwin && !ios && !server

#import <Cocoa/Cocoa.h>
#import <CoreServices/CoreServices.h>

extern void ravenLoginLaunchDetected(uintptr_t handle, int automatic);

// Capture the Apple event synchronously, while Cocoa is still handling launch.
// Wails forwards its application events to Go asynchronously, after this context
// may have gone away. SMAppService.mainAppService doesn't pass custom arguments.
@interface RavenLoginLaunchObserver : NSObject
@property(nonatomic) uintptr_t handle;
- (void)didFinishLaunching:(NSNotification *)notification;
@end

@implementation RavenLoginLaunchObserver
- (void)didFinishLaunching:(NSNotification *)notification {
    NSAppleEventDescriptor *event = NSAppleEventManager.sharedAppleEventManager.currentAppleEvent;
    BOOL automatic = event.eventID == kAEOpenApplication &&
        [[event paramDescriptorForKeyword:keyAEPropData] enumCodeValue] == keyAELaunchedAsLogInItem;
    [[NSNotificationCenter defaultCenter] removeObserver:self];
    ravenLoginLaunchDetected(self.handle, automatic ? 1 : 0);
}
@end

void ravenPrepareLoginDetection(uintptr_t handle) {
    static RavenLoginLaunchObserver *observer;
    observer = [RavenLoginLaunchObserver new];
    observer.handle = handle;
    [[NSNotificationCenter defaultCenter] addObserver:observer
        selector:@selector(didFinishLaunching:)
        name:NSApplicationDidFinishLaunchingNotification object:nil];
}

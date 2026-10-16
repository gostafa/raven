#import <Foundation/Foundation.h>
#import <AudioToolbox/AudioToolbox.h>

// Compile: clang -fobjc-arc -framework Foundation -framework AudioToolbox \
//   scripts/check-notification-sound.m -o /tmp/check-notification-sound
// Run with a packaged .app path to catch missing sounds and decoder failures.
int main(int argc, const char **argv) {
    if (argc != 2) {
        fprintf(stderr, "Usage: check-notification-sound /absolute/path/to/raven.app\n");
        return 2;
    }
    @autoreleasepool {
        NSString *path = [[NSString stringWithUTF8String:argv[1]] stringByStandardizingPath];
        if (!path.isAbsolutePath) {
            path = [NSFileManager.defaultManager.currentDirectoryPath stringByAppendingPathComponent:path];
        }
        NSBundle *bundle = [NSBundle bundleWithPath:path];
        NSURL *sound = [bundle URLForResource:@"raven" withExtension:@"wav"];
        if (!sound) {
            fprintf(stderr, "FAIL: macOS bundle lookup cannot find raven.wav\n");
            return 1;
        }
        SystemSoundID soundID = 0;
        OSStatus status = AudioServicesCreateSystemSoundID((__bridge CFURLRef)sound, &soundID);
        if (status != noErr) {
            fprintf(stderr, "FAIL: system alert decoder rejected raven.wav: %d\n", (int)status);
            return 1;
        }
        AudioServicesDisposeSystemSoundID(soundID);
        printf("PASS: bundle lookup and system alert decoding: %s\n", sound.path.UTF8String);
    }
    return 0;
}

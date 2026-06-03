// NOTE: this file is gated to macOS purely by its _darwin filename suffix — the
// Go toolchain does not read //go:build constraints inside .m/.c/.h files, so no
// build-tag line is used here (it would be inert and misleading).
//
// Objective-C side of the macOS "open document" handler. Kept in its own .m
// file (rather than the cgo preamble) so the YonOpenFilesHandler class is
// compiled exactly once — defining an @implementation in the preamble emits it
// into every cgo translation unit and the linker rejects the duplicate symbol.
// The _darwin suffix limits this file to macOS builds.

#import <Cocoa/Cocoa.h>
#import "_cgo_export.h" // declares yonOpenFile (the exported Go function)

@interface YonOpenFilesHandler : NSObject
- (void)handleOpenDocs:(NSAppleEventDescriptor *)event
             withReply:(NSAppleEventDescriptor *)reply;
@end

@implementation YonOpenFilesHandler
- (void)handleOpenDocs:(NSAppleEventDescriptor *)event
             withReply:(NSAppleEventDescriptor *)reply {
    NSAppleEventDescriptor *list = [event paramDescriptorForKeyword:keyDirectObject];
    NSInteger count = [list numberOfItems];
    // Apple Event item indices are 1-based.
    for (NSInteger i = 1; i <= count; i++) {
        NSAppleEventDescriptor *item = [list descriptorAtIndex:i];
        // Items arrive as alias or file-URL descriptors; coerce to typeFileURL
        // so we get a real path regardless of which form Finder sent.
        NSAppleEventDescriptor *fileURL = [item coerceToDescriptorType:typeFileURL];
        if (fileURL == nil) {
            continue;
        }
        NSURL *url = [NSURL URLWithDataRepresentation:[fileURL data] relativeToURL:nil];
        NSString *path = [url path];
        if (path != nil) {
            yonOpenFile((char *)[path UTF8String]);
        } else {
            // Log dropped items so a "double-click opened nothing" report is
            // diagnosable rather than silent (e.g. an unresolvable alias item).
            NSLog(@"yon: could not resolve open-document item %ld to a path", (long)i);
        }
    }
}
@end

static YonOpenFilesHandler *yonHandler = nil;

// installOpenFilesHandler (re)registers our handler for kAEOpenDocuments. It is
// safe to call more than once and intentionally re-registers each time: AppKit
// installs its own default handler for this event during app launch, so a
// handler set before the run loop starts gets clobbered. Callers register once
// early (best effort for a cold launch) and again after the app has started, so
// ours is the one that survives.
void installOpenFilesHandler(void) {
    if (yonHandler == nil) {
        yonHandler = [[YonOpenFilesHandler alloc] init];
    }
    [[NSAppleEventManager sharedAppleEventManager]
        setEventHandler:yonHandler
            andSelector:@selector(handleOpenDocs:withReply:)
          forEventClass:kCoreEventClass
             andEventID:kAEOpenDocuments];
}

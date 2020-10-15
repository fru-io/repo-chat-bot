package pkg

import (
	"fmt"

	siteapi "github.com/drud/ddev-live-sdk/golang/pkg/site/apis/site/v1beta1"
)

// These strings contain supported `/ddev-live-*` commands in PR/MR comments
const (
	// Command prefix
	commandPrefix = "/ddev-live-"

	// Ping/pong
	ping = "/ddev-live-ping"

	// Create preview site
	previewSite = "/ddev-live-preview-site"
)

// These strings contain responses for `/ddev-live-*` commands in PR/MR comments
const (
	// Pong
	pong = "ddev-live-pong"

	// Generic error when site cloning failed. We don't want to expose internal details on PR comments,
	// logs will contain more information about what failed
	previewGenericError = "**Internal error** creating preview sites"

	// Creation of `SiteClone` failed for a specific origin site,
	// logs will contain more information about what failed
	previewSiteError = "**Internal error** creating preview site from origin site `%v`"

	// Each clone site requires origin site, for `/ddev-live-preview-site` on a PR we determine origin
	// by looking at SiteImageSource if any site references the destination branch and repository.
	// This error is displayed to user when no valid origin site for cloning to create preview site from exists.
	previewSiteNoOrigin = "**Unable to create preview site**, no origin referencing branch `%v` found"

	// Creation of `SiteClone` succeeded, responding back to user that site cloning is in progress
	// using referenced origin site
	previewCreatingMsg = `**Creating preview site** ` + "`%v`" + ` in ` + "`%v`" + `. This will be kept up to date with site's current status as well as a preview URL once the site is fully available. You can also use the ` + "`ddev-live`" + ` CLI to get more information about the preview site creation progress:
` + "```" + `
$ ddev-live describe clone %v
$ ddev-live describe site %v
` + "```"
)

func previewCreating(sc *siteapi.SiteClone) string {
	msg := fmt.Sprintf(previewCreatingMsg, sc.Name, sc.Namespace, sc.Name, sc.Spec.Clone.Name)
	if err := sc.Error(); err != nil {
		return fmt.Sprintf("%v\n**Failed:** %v", msg, err)
	}
	_, ready := sc.Ready()
	return fmt.Sprintf("%v\n**Status:** %v", msg, ready)
}

package pkg

import (
	"fmt"
	"strings"

	common "github.com/drud/cms-common/api/v1beta1"
	siteapi "github.com/drud/ddev-live-sdk/golang/pkg/site/apis/site/v1beta1"
	typo3api "github.com/drud/ddev-live-sdk/golang/pkg/typo3/apis/cms/v1beta1"
)

// These strings contain supported `/ddev-live-*` commands in PR/MR comments
const (
	// Command prefix
	commandPrefix = "/ddev-live-"

	// Ping/pong
	Ping = "/ddev-live-ping"

	// Print help message for users calling the command
	Help = "/ddev-live-help"

	// Print help message on PR open, only when applicable
	HelpOnPROpen = "/ddev-live-help-on-pr-open"

	// Create preview site
	PreviewSite = "/ddev-live-preview-site"

	// Delete preview site, always provides verbose response event when no site exists
	DeletePreviewSite = "/ddev-live-delete-preview-site"

	// Close preview site, provides output only in case a preview site existed. This is for PR closing.
	ClosePreviewSite = "/ddev-live-close-preview-site"
)

// These strings contain responses for `/ddev-live-*` commands in PR/MR comments
const (
	// Pong
	pong = "ddev-live-pong"

	// Help message how to use the repo chat bot
	helpResponse = "**DDEV-Live preview bot** available commands\n" +
		"```\n" +
		Help + " - displays this help message\n" +
		PreviewSite + " - creates preview site cloning origin branch\n" +
		DeletePreviewSite + " - deletes the preview site\n" +
		"```"

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
	previewCreatingMsg = "`%v`" + ` in ` + "`%v`" + `. This will be kept up to date with site's current status. You can also use the ` + "`ddev-live`" + ` CLI to get more information about the preview site creation progress:
` + "```" + `
$ ddev-live describe clone %v
$ ddev-live describe site %v
` + "```"

	// Deletion of `SiteClone` failed for a specific origin site,
	// logs will contain more information about what failed
	deleteSiteError = "**Internal error** failed to delete preview site `%v` in `%v`"

	// Deletion of `SiteClone` in progress
	deleteSite = "**Deleting preview site** `%v` in `%v`"

	// No site to be deleted
	deleteSiteNone = "**No preview site to be deleted**"

	// The `SiteClone` was deleted, child resources will be garbage collected
	deletedSite = "**Deleted preview site** `%v` in `%v`"
)

type siteStatus struct {
	conditions []common.Condition
	webStatus  common.WebStatus
}

func getCommonStatus(t3 *typo3api.Typo3Site) siteStatus {
	var conditions []common.Condition
	for _, c := range t3.Status.Conditions {
		conditions = append(conditions, common.Condition{
			Type:               common.ConditionType(c.Type),
			Status:             c.Status,
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}
	return siteStatus{conditions: conditions, webStatus: common.WebStatus(t3.Status.WebStatus)}
}

func previewCreating(sc *siteapi.SiteClone, site siteStatus) string {
	msg := fmt.Sprintf(previewCreatingMsg, sc.Name, sc.Namespace, sc.Name, sc.Spec.Clone.Name)
	if err := sc.Error(); err != nil {
		return fmt.Sprintf("**Creating preview site** %v\n**Failed:** %v", msg, err)
	}
	scReady, scReadyMsg := sc.Ready()
	if !scReady {
		return fmt.Sprintf("**Creating preview site** %v\n**Status:** %v", msg, scReadyMsg)
	}
	sReady, _ := common.Ready(site.conditions)
	sn := fmt.Sprintf("%v/%v", sc.Namespace, sc.Spec.Clone.Name)
	if !sReady {
		return fmt.Sprintf("**Creating preview site** %v\n**Status:** Site %v is getting ready", msg, sn)
	}
	if len(site.webStatus.URLs) == 0 {
		return fmt.Sprintf("**Creating preview site** %v\n**Status:** Site %v is waiting for preview URL", msg, sn)
	}
	return fmt.Sprintf("**Preview site created** %v\n**Preview URL:** %v", msg, site.webStatus.URLs[0])
}

func isBot(msg string) bool {
	if msg == helpResponse {
		return true
	}
	if strings.HasPrefix(msg, deleteSiteNone) {
		return true
	}
	return false
}

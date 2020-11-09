# GitHub and GitLab PR/MR bot for DDEV-Live

This repository contains common functionality for GitHub and GitLab bot. DDEV-Live users will be able to 
communicate with ddev-live platform using PR or MR comments formatted as commands. Initial scope is limited to only 
preview sites, also known as site cloning for PRs/MRs.

#### Core GitHub/GitLab bot tasks described in:
* https://github.com/drud/ddev-live/issues/517 - GitHub bot
* https://github.com/drud/ddev-live/issues/516 - GitLab support

#### Initial set of commands, responses and triggers we would like to support:
* https://github.com/drud/ddev-live/issues/512 - command: create a preview site
* https://github.com/drud/ddev-live/issues/511 - notification: preview site is building
* https://github.com/drud/ddev-live/issues/510 - trigger: delete preview site on PR/MR close
* https://github.com/drud/ddev-live/issues/513 - notification: preview site deleted
* https://github.com/drud/ddev-live/issues/514 - notification: preview site build errors
* https://github.com/drud/ddev-live/issues/515 - notification: preview site build completed

## Integration configuration

For GitHub, this requires read and write access to `issues` and `pull requests` scope, the bot will be available on
repositories selected in the GitHub Apps user installation.

For GitLab, this requires `api` scope on the issued PAT and the bot will be available on all repositories referenced
by user created `SiteImageSource`.

#### Basic test

The repo-chat-bot supports simple ping/pong command response to test the integration:

Comment on PR/MR with ddev-live command:
```
/ddev-live-ping
```

Bot should promptly respond in a following comment:
```
ddev-live-pong
```

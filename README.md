# DDEV-Live Repo Chat Bot Library

![DDEV-Live logo](docs/ddev_logo.png)

This repository provides common functionality for GitHub and GitLab bot which enables DDEV-Live users
to communicate with the platform using comments on PRs or MRs formatted as `commands`.

The library works in conjunction with [ddev-live-sdk](https://github.com/drud/ddev-live-sdk) and expects
the DDEV-Live platform to be deployed on a kubernetes cluster.

## Usage

This is currently used by [github-operator](https://github.com/drud/github-operator) and 
[gitlab-webhook-server](https://github.com/drud/gitlab-webhook-server). They both serve as webhook servers
watching on events from respective git providers and respond back with information to the user.

Currently supported commands are:

| Command | Explanation |
|---------|-------------|
|`/ddev-live-preview-site` | Provision new preview site or report back the most current status of the preview site.|
|`/ddev-live-delete-preview-site` | Manually trigger deletion of the preview site when you no longer require it.|
|`/ddev-live-help` | Display usage and help message how to interact with DDEV-Live bot.|

There are also automatic triggers built in:
* **New PRs created**, that can be used for preview site creation - triggers `/ddev-live-help` to provide basic information for the user
* **PR is closed** both by merge or without merging, the preview site is automatically deleted as if user called `/ddev-live-delete-preview-site`

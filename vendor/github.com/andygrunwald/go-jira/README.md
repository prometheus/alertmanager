# go-jira

[![GoDoc](https://godoc.org/github.com/andygrunwald/go-jira?status.svg)](https://godoc.org/github.com/andygrunwald/go-jira)
[![Build Status](https://travis-ci.org/andygrunwald/go-jira.svg?branch=master)](https://travis-ci.org/andygrunwald/go-jira)
[![Go Report Card](https://goreportcard.com/badge/github.com/andygrunwald/go-jira)](https://goreportcard.com/report/github.com/andygrunwald/go-jira)

[Go](https://golang.org/) client library for [Atlassian JIRA](https://www.atlassian.com/software/jira).

![Go client library for Atlassian JIRA](./img/go-jira-compressed.png "Go client library for Atlassian JIRA.")

## Features

* Authentication (HTTP Basic, OAuth, Session Cookie)
* Create and receive issues
* Create and retrieve issue transitions (status updates)
* Call every API endpoint of the JIRA, even it is not directly implemented in this library

This package is not JIRA API complete (yet), but you can call every API endpoint you want. See [Call a not implemented API endpoint](#call-a-not-implemented-api-endpoint) how to do this. For all possible API endpoints of JRIA have a look at [latest JIRA REST API documentation](https://docs.atlassian.com/jira/REST/latest/).

## Compatible JIRA versions

This package was tested against JIRA v6.3.4 and v7.1.2.

## Installation

It is go gettable

    $ go get github.com/andygrunwald/go-jira

For stable versions you can use one of our tags with [gopkg.in](http://labix.org/gopkg.in). E.g.

```go
package main

import (
	jira "gopkg.in/andygrunwald/go-jira.v1"
)
...
```

(optional) to run unit / example tests:

    $ cd $GOPATH/src/github.com/andygrunwald/go-jira
    $ go test -v ./...

## API

Please have a look at the [GoDoc documentation](https://godoc.org/github.com/andygrunwald/go-jira) for a detailed API description.

The [latest JIRA REST API documentation](https://docs.atlassian.com/jira/REST/latest/) was the base document for this package.

## Examples

Further a few examples how the API can be used.
A few more examples are available in the [GoDoc examples section](https://godoc.org/github.com/andygrunwald/go-jira#pkg-examples).

### Get a single issue

Lets retrieve [MESOS-3325](https://issues.apache.org/jira/browse/MESOS-3325) from the [Apache Mesos](http://mesos.apache.org/) project.

```go
package main

import (
	"fmt"
	"github.com/andygrunwald/go-jira"
)

func main() {
	jiraClient, _ := jira.NewClient(nil, "https://issues.apache.org/jira/")
	issue, _, _ := jiraClient.Issue.Get("MESOS-3325", nil)

	fmt.Printf("%s: %+v\n", issue.Key, issue.Fields.Summary)
	fmt.Printf("Type: %s\n", issue.Fields.Type.Name)
	fmt.Printf("Priority: %s\n", issue.Fields.Priority.Name)

	// MESOS-3325: Running mesos-slave@0.23 in a container causes slave to be lost after a restart
	// Type: Bug
	// Priority: Critical
}
```

### Authenticate with jira

Some actions require an authenticated user.

#### Authenticate with basic auth

Here is an example with a basic auth authentification.

```go
package main

import (
	"fmt"
	"github.com/andygrunwald/go-jira"
)

func main() {
	jiraClient, err := jira.NewClient(nil, "https://your.jira-instance.com/")
	if err != nil {
		panic(err)
	}
	jiraClient.Authentication.SetBasicAuth("username", "password")

	issue, _, err := jiraClient.Issue.Get("SYS-5156", nil)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s: %+v\n", issue.Key, issue.Fields.Summary)
}
```

#### Authenticate with session cookie

Here is an example with a session cookie authentification.

```go
package main

import (
	"fmt"
	"github.com/andygrunwald/go-jira"
)

func main() {
	jiraClient, err := jira.NewClient(nil, "https://your.jira-instance.com/")
	if err != nil {
		panic(err)
	}

	res, err := jiraClient.Authentication.AcquireSessionCookie("username", "password")
	if err != nil || res == false {
		fmt.Printf("Result: %v\n", res)
		panic(err)
	}

	issue, _, err := jiraClient.Issue.Get("SYS-5156", nil)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s: %+v\n", issue.Key, issue.Fields.Summary)
}
```

#### Authenticate with OAuth

If you want to connect via OAuth to your JIRA Cloud instance checkout the [example of using OAuth authentication with JIRA in Go](https://gist.github.com/Lupus/edafe9a7c5c6b13407293d795442fe67) by [@Lupus](https://github.com/Lupus).

For more details have a look at the [issue #56](https://github.com/andygrunwald/go-jira/issues/56).

### Create an issue

Example how to create an issue.

```go
package main

import (
	"fmt"
	"github.com/andygrunwald/go-jira"
)

func main() {
	jiraClient, err := jira.NewClient(nil, "https://your.jira-instance.com/")
	if err != nil {
		panic(err)
	}

	res, err := jiraClient.Authentication.AcquireSessionCookie("username", "password")
	if err != nil || res == false {
		fmt.Printf("Result: %v\n", res)
		panic(err)
	}

	i := jira.Issue{
		Fields: &jira.IssueFields{
			Assignee: &jira.User{
				Name: "myuser",
			},
			Reporter: &jira.User{
				Name: "youruser",
			},
			Description: "Test Issue",
			Type: jira.IssueType{
				Name: "Bug",
			},
			Project: jira.Project{
				Key: "PROJ1",
			},
			Summary: "Just a demo issue",
		},
	}
	issue, _, err := jiraClient.Issue.Create(&i)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s: %+v\n", issue.Key, issue.Fields.Summary)
}
```

### Call a not implemented API endpoint

Not all API endpoints of the JIRA API are implemented into *go-jira*.
But you can call them anyway:
Lets get all public projects of [Atlassian`s JIRA instance](https://jira.atlassian.com/).

```go
package main

import (
	"fmt"
	"github.com/andygrunwald/go-jira"
)

func main() {
	jiraClient, _ := jira.NewClient(nil, "https://jira.atlassian.com/")
	req, _ := jiraClient.NewRequest("GET", "/rest/api/2/project", nil)

	projects := new([]jira.Project)
	_, err := jiraClient.Do(req, projects)
	if err != nil {
		panic(err)
	}

	for _, project := range *projects {
		fmt.Printf("%s: %s\n", project.Key, project.Name)
	}

	// ...
	// BAM: Bamboo
	// BAMJ: Bamboo JIRA Plugin
	// CLOV: Clover
	// CONF: Confluence
	// ...
}
```

## Implementations

* [andygrunwald/jitic](https://github.com/andygrunwald/jitic) - The JIRA Ticket Checker

## Code structure

The code structure of this package was inspired by [google/go-github](https://github.com/google/go-github).

There is one main part (the client).
Based on this main client the other endpoints, like Issues or Authentication are extracted in services. E.g. `IssueService` or `AuthenticationService`.
These services own a responsibility of the single endpoints / usecases of JIRA.

## Contribution

Contribution, in any kind of way, is highly welcome!
It doesn't matter if you are not able to write code.
Creating issues or holding talks and help other people to use [go-jira](https://github.com/andygrunwald/go-jira) is contribution, too!
A few examples:

* Correct typos in the README / documentation
* Reporting bugs
* Implement a new feature or endpoint
* Sharing the love if [go-jira](https://github.com/andygrunwald/go-jira) and help people to get use to it

If you are new to pull requests, checkout [Collaborating on projects using issues and pull requests / Creating a pull request](https://help.github.com/articles/creating-a-pull-request/).

## License

This project is released under the terms of the [MIT license](http://en.wikipedia.org/wiki/MIT_License).

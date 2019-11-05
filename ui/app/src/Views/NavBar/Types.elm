module Views.NavBar.Types exposing (SingleTab, Tab(..), alertsTab, noneTab, silencesTab, statusTab, tabs)


type alias DropdownTab =
    { name : String
    , lTab : List SingleTab
    }


type alias SingleTab =
    { link : String
    , name : String
    }


type Tab
    = ST SingleTab
    | DT DropdownTab


alertsTab : Tab
alertsTab =
    ST
        { link = "#/alerts", name = "Alerts" }


silencesTab : Tab
silencesTab =
    ST
        { link = "#/silences", name = "Silences" }


statusTab : Tab
statusTab =
    ST
        { link = "#/status", name = "Status" }


wikiTab : Tab
wikiTab =
    ST
        { link = "https://ns1inc.atlassian.net/wiki/spaces/infrastructure/", name = "Wiki" }


grafanaTab : Tab
grafanaTab =
    ST
        { link = "http://dash.nsone.co:3000/", name = "Grafana" }


jenkinsTab : Tab
jenkinsTab =
    ST
        { link = "http://jenkins.nsone.co:8080/blue/organizations/jenkins/pipelines/", name = "Jenkins" }


billingTsdbRootTab : SingleTab
billingTsdbRootTab =
    { link = "http://billing-metrics.nsone.co/", name = "TSDB" }


billingTsdbLBTab : SingleTab
billingTsdbLBTab =
    { link = "http://billing-metrics.nsone.co:9090/haproxy?stats", name = "TSDB LB" }


billingTsdbCdhTab : SingleTab
billingTsdbCdhTab =
    { link = "http://cdh01.lga08.nsone.co:7180/cmf/home", name = "CDH" }


billingTsdbDropdownTab : DropdownTab
billingTsdbDropdownTab =
    { name = "Billing TSDB"
    , lTab = [ billingTsdbRootTab, billingTsdbLBTab, billingTsdbCdhTab ]
    }


noneTab : Tab
noneTab =
    ST
        { link = "", name = "" }


tabs : List Tab
tabs =
    [ alertsTab, silencesTab, statusTab, wikiTab, grafanaTab, jenkinsTab, DT billingTsdbDropdownTab ]

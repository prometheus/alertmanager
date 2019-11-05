module Views.NavBar.Types exposing (SingleTab, Tab(..), alertsTab, noneTab, silencesTab, statusTab, tabs)


type alias DropdownTab =
    { name : String
    , lTab : List Tab
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


billingTsdbRootTab : Tab
billingTsdbRootTab =
    ST
        { link = "http://billing-metrics.nsone.co/", name = "TSDB" }


billingTsdbLBTab : Tab
billingTsdbLBTab =
    ST
        { link = "http://billing-metrics.nsone.co:9090/haproxy?stats", name = "TSDB LB" }


billingTsdbCdhTab : Tab
billingTsdbCdhTab =
    ST
        { link = "http://cdh01.lga08.nsone.co:7180/cmf/home", name = "CDH" }


billingTsdbDropdownTab : Tab
billingTsdbDropdownTab =
    DT
        { name = "Billing TSDB"
        , lTab = [ billingTsdbRootTab, billingTsdbLBTab, billingTsdbCdhTab ]
        }


ldapTab : Tab
ldapTab =
    ST
        { link = "http://misc02.lga08.nsone.co/", name = "LDAP" }


openvasTab : Tab
openvasTab =
    ST
        { link = "https://vm01.lga08.nsone.co", name = "OpenVAS" }


inventoryTab : Tab
inventoryTab =
    ST
        { link = "http://jenkins.nsone.co:8080/job/inventory/lastSuccessfulBuild/artifact/inventory.html", name = "Inventory" }


ubersmithTab : Tab
ubersmithTab =
    ST
        { link = "https://ubersmith.nsone.co", name = "Ubersmith" }


miscDropdownTab : Tab
miscDropdownTab =
    DT
        { name = "Misc"
        , lTab = [ ldapTab, openvasTab, inventoryTab, ubersmithTab ]
        }


mongoCoreTab : Tab
mongoCoreTab =
    ST
        { link = "http://compute03.lga08.nsone.co:9090/haproxy?stats", name = "Mongo Core" }


mongoEdgeTab : Tab
mongoEdgeTab =
    ST
        { link = "http://compute03.lga08.nsone.co:9091/haproxy?stats", name = "Mongo Edge" }


redundantMongoTab : Tab
redundantMongoTab =
    ST
        { link = "http://mconnector04.lga08.nsone.co:9090/haproxy?stats", name = "Redundant Mongo" }


mongoLBsDropdownTab : Tab
mongoLBsDropdownTab =
    DT
        { name = "Mongo LBs"
        , lTab = [ mongoCoreTab, mongoEdgeTab, redundantMongoTab ]
        }


nfsenTab : Tab
nfsenTab =
    ST
        { link = "http://netstats02.lga02.nsone.co/nfsen/", name = "NfSen" }


observiumTab : Tab
observiumTab =
    ST
        { link = "https://observium.nsone.co", name = "Observium" }


rancidTab : Tab
rancidTab =
    ST
        { link = "http://rancid01.lga08.nsone.co/websvn/listing.php?repname=repos+nsone", name = "Rancid" }


networkingDropdownTab : Tab
networkingDropdownTab =
    DT
        { name = "Networking"
        , lTab = [ nfsenTab, observiumTab, rancidTab ]
        }


kibanaTab : Tab
kibanaTab =
    ST
        { link = "http://logs.nsone.co:5601", name = "Kibana" }


elkStatusTab : Tab
elkStatusTab =
    ST
        { link = "http://es.nsone.co/#/overview?host=nsone-elasticsearch", name = "Status" }


nginxTab : Tab
nginxTab =
    ST
        { link = "http://logs.nsone.co:5601/app/kibana#/dashboard/Nginx", name = "Nginx" }


netflowsTab : Tab
netflowsTab =
    ST
        { link = "http://logs.nsone.co:5601/app/kibana#/dashboard/Packets", name = "Netflows" }


pktvisorTab : Tab
pktvisorTab =
    ST
        { link = "http://logs.nsone.co:5601/app/kibana#/dashboard/Pktvisor", name = "Pktvisor" }


elkDropdownTab : Tab
elkDropdownTab =
    DT
        { name = "ELK"
        , lTab = [ kibanaTab, elkStatusTab, nginxTab, netflowsTab, pktvisorTab ]
        }


noneTab : Tab
noneTab =
    ST
        { link = "", name = "" }


tabs : List Tab
tabs =
    [ alertsTab, silencesTab, statusTab, wikiTab, grafanaTab, jenkinsTab, miscDropdownTab, billingTsdbDropdownTab, mongoLBsDropdownTab, networkingDropdownTab, elkDropdownTab ]

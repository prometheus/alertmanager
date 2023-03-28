module Views.NavBar.Types exposing (Tab, alertsTab, noneTab, settingsTab, silencesTab, statusTab, tabs)


type alias Tab =
    { link : String
    , name : String
    }


alertsTab : Tab
alertsTab =
    { link = "#/alerts", name = "Alerts" }


silencesTab : Tab
silencesTab =
    { link = "#/silences", name = "Silences" }


statusTab : Tab
statusTab =
    { link = "#/status", name = "Status" }


settingsTab : Tab
settingsTab =
    { link = "#/settings", name = "Settings" }


helpTab : Tab
helpTab =
    { link = "https://prometheus.io/docs/alerting/alertmanager/", name = "Help" }


noneTab : Tab
noneTab =
    { link = "", name = "" }


tabs : List Tab
tabs =
    [ alertsTab, silencesTab, statusTab, settingsTab, helpTab ]

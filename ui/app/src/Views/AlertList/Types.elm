module Views.AlertList.Types exposing (AlertListMsg(..), Model, Tab(..), initAlertList)

import Utils.Types exposing (ApiData(Initial))
import Alerts.Types exposing (Alert)
import Views.FilterBar.Types as FilterBar
import Views.GroupBar.Types as GroupBar


type AlertListMsg
    = AlertsFetched (ApiData (List Alert))
    | FetchAlerts
    | MsgForFilterBar FilterBar.Msg
    | MsgForGroupBar GroupBar.Msg
    | ToggleSilenced Bool
    | SetActive (Maybe String)
    | SetTab Tab


type Tab
    = FilterTab
    | GroupTab


type alias Model =
    { alerts : ApiData (List Alert)
    , groupBar : GroupBar.Model
    , filterBar : FilterBar.Model
    , tab : Tab
    , activeId : Maybe String
    }


initAlertList : Model
initAlertList =
    { alerts = Initial
    , groupBar = GroupBar.initGroupBar
    , filterBar = FilterBar.initFilterBar
    , tab = FilterTab
    , activeId = Nothing
    }

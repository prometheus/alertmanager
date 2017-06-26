module Views.AlertList.Types exposing (AlertListMsg(..), Model, Tab(..), initAlertList)

import Alerts.Types exposing (Alert)
import Utils.Types exposing (ApiData(Initial))
import Views.FilterBar.Types as FilterBar
import Views.GroupBar.Types as GroupBar


type AlertListMsg
    = AlertsFetched (ApiData (List Alert))
    | ReceiversFetched (ApiData (List String))
    | ToggleReceivers Bool
    | SelectReceiver (Maybe String)
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
    , receivers : List String
    , showRecievers : Bool
    , groupBar : GroupBar.Model
    , filterBar : FilterBar.Model
    , tab : Tab
    , activeId : Maybe String
    }


initAlertList : Model
initAlertList =
    { alerts = Initial
    , receivers = []
    , showRecievers = False
    , groupBar = GroupBar.initGroupBar
    , filterBar = FilterBar.initFilterBar
    , tab = FilterTab
    , activeId = Nothing
    }

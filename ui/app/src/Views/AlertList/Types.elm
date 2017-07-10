module Views.AlertList.Types exposing (AlertListMsg(..), Model, Tab(..), initAlertList)

import Alerts.Types exposing (Alert)
import Utils.Types exposing (ApiData(Initial))
import Views.FilterBar.Types as FilterBar
import Views.GroupBar.Types as GroupBar
import Views.ReceiverBar.Types as ReceiverBar


type AlertListMsg
    = AlertsFetched (ApiData (List Alert))
    | FetchAlerts
    | MsgForReceiverBar ReceiverBar.Msg
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
    , receiverBar : ReceiverBar.Model
    , groupBar : GroupBar.Model
    , filterBar : FilterBar.Model
    , tab : Tab
    , activeId : Maybe String
    }


initAlertList : Model
initAlertList =
    { alerts = Initial
    , receiverBar = ReceiverBar.initReceiverBar
    , groupBar = GroupBar.initGroupBar
    , filterBar = FilterBar.initFilterBar
    , tab = FilterTab
    , activeId = Nothing
    }

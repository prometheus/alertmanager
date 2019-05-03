module Views.AlertList.Types exposing
    ( AlertListMsg(..)
    , Model
    , Tab(..)
    , initAlertList
    )

import Browser.Navigation exposing (Key)
import Data.AlertGroup exposing (AlertGroup)
import Data.GettableAlert exposing (GettableAlert)
import Utils.Types exposing (ApiData(..), Labels)
import Views.FilterBar.Types as FilterBar
import Views.GroupBar.Types as GroupBar
import Views.ReceiverBar.Types as ReceiverBar


type AlertListMsg
    = AlertsFetched (ApiData (List GettableAlert))
    | AlertGroupsFetched (ApiData (List AlertGroup))
    | FetchAlerts
    | MsgForReceiverBar ReceiverBar.Msg
    | MsgForFilterBar FilterBar.Msg
    | MsgForGroupBar GroupBar.Msg
    | ToggleSilenced Bool
    | ToggleInhibited Bool
    | SetActive (Maybe String)
    | SetGroup (Maybe Labels)
    | SetTab Tab


type Tab
    = FilterTab
    | GroupTab


type alias Model =
    { alerts : ApiData (List GettableAlert)
    , alertGroups : ApiData (List AlertGroup)
    , receiverBar : ReceiverBar.Model
    , groupBar : GroupBar.Model
    , filterBar : FilterBar.Model
    , tab : Tab
    , activeId : Maybe String
    , activeLabels : Maybe Labels
    , key : Key
    }


initAlertList : Key -> Model
initAlertList key =
    { alerts = Initial
    , alertGroups = Initial
    , receiverBar = ReceiverBar.initReceiverBar key
    , groupBar = GroupBar.initGroupBar key
    , filterBar = FilterBar.initFilterBar key
    , tab = FilterTab
    , activeId = Nothing
    , activeLabels = Nothing
    , key = key
    }

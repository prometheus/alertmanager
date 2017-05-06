module Views.AlertList.Types exposing (AlertListMsg(..), Model, initAlertList)

import Utils.Types exposing (ApiData, ApiResponse(Loading))
import Alerts.Types exposing (Alert)
import Views.FilterBar.Types as FilterBar
import Utils.Filter exposing (Filter)
import Set exposing (Set)
import Views.AutoComplete.Types as AutoComplete


type AlertListMsg
    = AlertsFetched (ApiData (List Alert))
    | FetchAlerts
    | MsgForFilterBar FilterBar.Msg
    | MsgForAutoComplete AutoComplete.Msg


type alias Model =
    { alerts : ApiData (List Alert)
    , autoComplete : AutoComplete.Model
    , filterBar : FilterBar.Model
    }


initAlertList : Model
initAlertList =
    { alerts = Loading
    , autoComplete = AutoComplete.initAutoComplete
    , filterBar = FilterBar.initFilterBar
    }

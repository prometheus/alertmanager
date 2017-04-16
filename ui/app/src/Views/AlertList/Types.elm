module Views.AlertList.Types exposing (AlertListMsg(..), Model, initAlertList)

import Utils.Types exposing (ApiData, Filter, ApiResponse(Loading))
import Alerts.Types exposing (Alert, AlertGroup)
import Utils.Filter


type AlertListMsg
    = AlertGroupsFetch (ApiData (List AlertGroup))
    | FetchAlertGroups
    | AddFilterMatcher Bool Utils.Filter.Matcher
    | DeleteFilterMatcher Bool Utils.Filter.Matcher
    | PressingBackspace Bool
    | UpdateMatcherText String


type alias Model =
    { alertGroups : ApiData (List AlertGroup)
    , matchers : List Utils.Filter.Matcher
    , backspacePressed : Bool
    , matcherText : String
    }


initAlertList : Model
initAlertList =
    { alertGroups = Loading
    , matchers = []
    , backspacePressed = False
    , matcherText = ""
    }

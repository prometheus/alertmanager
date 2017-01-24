module Types exposing (..)

-- External Imports

import Alerts.Types exposing (AlertGroup, AlertsMsg, Alert)
import Http exposing (Error)
import ISO8601
import Time


-- Internal Imports
-- Types


type alias Model =
    { silences : List Silence
    , silence : Silence
    , alertGroups : List AlertGroup
    , route : Route
    , error : String
    , loading : Bool
    }


type alias Silence =
    { id : Int
    , createdBy : String
    , comment : String
    , startsAt : Time
    , endsAt : Time
    , createdAt : Time
    , matchers : List Matcher
    }


type alias Time =
    { t : ISO8601.Time
    , s : String
    , valid : Bool
    }


type alias Matcher =
    { name : String
    , value : String
    , isRegex : Bool
    }


type Msg
    = SilenceFetch (Result Http.Error Silence)
    | SilencesFetch (Result Http.Error (List Silence))
    | SilenceCreate (Result Http.Error Int)
    | SilenceDestroy (Result Http.Error Int)
    | FetchSilences
    | FetchSilence Int
    | NewSilence
    | EditSilence Int
    | CreateSilence Silence
    | CreateSilenceFromAlert Alert
    | UpdateLoading Bool
    | DestroySilence Silence
    | NavigateToAlerts Alerts.Types.Route
    | Alerts AlertsMsg
    | RedirectAlerts
    | DeleteMatcher Matcher
    | AddMatcher
    | UpdateMatcherName Matcher String
    | UpdateMatcherValue Matcher String
    | UpdateMatcherRegex Matcher Bool
    | UpdateEndsAt String
    | UpdateStartsAt String
    | UpdateCreatedBy String
    | UpdateComment String
    | NewDefaultTimeRange Time.Time
    | Noop


type Route
    = SilencesRoute
    | NewSilenceRoute
    | SilenceRoute Int
    | EditSilenceRoute Int
    | AlertsRoute Alerts.Types.Route
    | TopLevel
    | NotFound

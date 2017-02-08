module Types exposing (..)

-- External Imports

import Alerts.Types exposing (AlertGroup, AlertsMsg, Alert)
import Http exposing (Error)
import ISO8601
import Time


-- Internal Imports
-- Types


type alias Model =
    { silences : ApiData (List Silence)
    , silence : ApiData Silence
    , alertGroups : List AlertGroup
    , route : Route
    , error : String
    }


type ApiResponse e a
    = NotAsked
    | Loading
    | Failure e
    | Success a


type alias ApiData a =
    ApiResponse Http.Error a


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
    = SilenceFetch (ApiData Silence)
    | SilencesFetch (ApiData (List Silence))
    | SilenceCreate (Result Http.Error Int)
    | SilenceDestroy (Result Http.Error Int)
    | FetchSilences
    | FetchSilence Int
    | NewSilence
    | EditSilence Int
    | CreateSilence Silence
    | CreateSilenceFromAlert Alert
    | DestroySilence Silence
    | NavigateToAlerts Alerts.Types.Route
    | Alerts AlertsMsg
    | RedirectAlerts
    | DeleteMatcher Silence Matcher
    | AddMatcher Silence
    | UpdateMatcherName Silence Matcher String
    | UpdateMatcherValue Silence Matcher String
    | UpdateMatcherRegex Silence Matcher Bool
    | UpdateEndsAt Silence String
    | UpdateStartsAt Silence String
    | UpdateCreatedBy Silence String
    | UpdateComment Silence String
    | NewDefaultTimeRange Silence Time.Time
    | Noop


type Route
    = SilencesRoute
    | NewSilenceRoute
    | SilenceRoute Int
    | EditSilenceRoute Int
    | AlertsRoute Alerts.Types.Route
    | TopLevel
    | NotFound

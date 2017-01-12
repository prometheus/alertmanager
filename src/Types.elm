module Types exposing (..)

-- External Imports

import Http exposing (Error)
import Time
import ISO8601


-- Internal Imports
-- Types


type alias Model =
    { silences : List Silence
    , silence : Silence
    , alertGroups : List AlertGroup
    , route : Route
    }


type alias Silence =
    { id : Int
    , createdBy : String
    , comment : String
    , startsAt : ISO8601.Time
    , endsAt : ISO8601.Time
    , createdAt : ISO8601.Time
    , matchers : List Matcher
    }


type alias AlertGroup =
    { blocks : List Block
    , labels : List ( String, String )
    }


type alias Alert =
    { annotations : List ( String, String )
    , labels : List ( String, String )
    , inhibited : Bool
    , silenceId : Maybe Int
    , silenced : Bool
    , startsAt : ISO8601.Time
    , generatorUrl : String
    }


type alias Block =
    { alerts : List Alert
    , routeOpts : RouteOpts
    }


type alias RouteOpts =
    { receiver : String }


type alias Matcher =
    { name : String
    , value : String
    , isRegex : Bool
    }


type Msg
    = SilenceFetch (Result Http.Error Silence)
    | SilencesFetch (Result Http.Error (List Silence))
    | FetchSilences
    | FetchSilence Int
    | NewSilence
    | EditSilence Int
    | AlertGroupsFetch (Result Http.Error (List AlertGroup))
    | FetchAlertGroups
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
    | SilenceFromAlert (List Matcher)
    | Noop (List Matcher)
    | NewDefaultTimeRange Time.Time


type Route
    = SilencesRoute
    | NewSilenceRoute
    | SilenceRoute Int
    | EditSilenceRoute Int
    | AlertGroupsRoute
    | TopLevel
    | NotFound

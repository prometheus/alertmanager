module Types exposing (..)

-- External Imports

import Http exposing (Error)


-- Internal Imports
-- Types


type alias Model =
    { silences : List Silence
    , silence : Silence
    , alerts : List Alert
    , alert : Alert
    , route : Route
    }


type alias Silence =
    { id : Int
    , createdBy : String
    , comment : String
    , startsAt : String
    , endsAt : String
    , createdAt : String
    , matchers : List Matcher
    }



-- TODO: Implement Alert.


type alias Alert =
    { id : String }



-- TODO: Implement Matcher.


type alias Matcher =
    { name : String
    , value : String
    , isRegex : Bool
    }


type Msg
    = SilenceFetchSucceed Silence
    | SilencesFetchSucceed (List Silence)
    | AlertFetchSucceed Alert
    | AlertsFetchSucceed (List Alert)
    | FetchFail Http.Error


type Route
    = SilencesRoute
    | SilenceRoute String
    | AlertsRoute
    | AlertRoute String
    | TopLevel
    | NotFound

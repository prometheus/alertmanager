module Types exposing (..)

-- External Imports

import Http exposing (Error)


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
    , startsAt : String
    , endsAt : String
    , createdAt : String
    , matchers : List Matcher
    }



-- TODO: Implement Alert.


type alias AlertGroup =
    { alerts : Maybe (List (List Alert))
    , labels : List ( String, String )
    }


type alias Alert =
    { annotations : List ( String, String )
    , labels : List ( String, String )
    , inhibited : Bool
    , silenced :
        Maybe Int
        -- TODO: See how to rename this on parsing from API to silenceId
    , startsAt : String
    , generatorUrl : String
    }


type alias Block =
    { alerts : List Alert }



-- TODO: Implement Matcher.


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
    | RedirectSilences


type Route
    = SilencesRoute
    | NewSilenceRoute
    | SilenceRoute Int
    | EditSilenceRoute Int
    | AlertGroupsRoute
    | TopLevel
    | NotFound

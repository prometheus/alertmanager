module Status.Types exposing (StatusMsg(..), StatusModel, StatusResponse)

import Http exposing (Error)


type StatusMsg
    = NewStatus (Result Http.Error StatusResponse)
    | GetStatus


type alias StatusModel =
    { response : Maybe StatusResponse
    }


type alias VersionInfo =
    { branch : String
    , buildDate : String
    , buildUser : String
    , goVersion : String
    , revision : String
    , version : String
    }


type alias StatusResponse =
    { status : String
    , uptime : String
    }

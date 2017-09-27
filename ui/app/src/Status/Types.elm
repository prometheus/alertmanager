module Status.Types exposing (..)

import Utils.Types exposing (Matcher)


type alias StatusResponse =
    { config : String
    , uptime : String
    , versionInfo : VersionInfo
    , meshStatus : Maybe MeshStatus
    , route : Route
    }


type alias VersionInfo =
    { branch : String
    , buildDate : String
    , buildUser : String
    , goVersion : String
    , revision : String
    , version : String
    }


type alias MeshStatus =
    { name : String
    , nickName : String
    , peers : List MeshPeer
    }


type alias MeshPeer =
    { name : String
    , nickName : String
    , uid : Int
    }


type alias Route =
    { receiver : Maybe String
    , group_by : Maybe (List String)
    , continue : Maybe Bool
    , matchers : List Matcher
    , group_wait : Maybe Int
    , group_interval : Maybe Int
    , repeat_interval : Maybe Int
    , routes : Maybe Routes
    , parent : Maybe Parent
    , x : Int
    , y : Int
    , mod : Int
    }


type Routes
    = Routes (List Route)


type Parent
    = Parent Route

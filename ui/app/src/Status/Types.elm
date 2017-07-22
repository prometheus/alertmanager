module Status.Types exposing (StatusResponse, VersionInfo, MeshStatus, MeshPeer)


type alias StatusResponse =
    { config : String
    , uptime : String
    , versionInfo : VersionInfo
    , meshStatus : Maybe MeshStatus
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

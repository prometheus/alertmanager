module Views.Settings.Types exposing (..)


type alias Model =
    { startOfWeek : Int
    }


type SettingsMsg
    = UpdateStartWeekAtMonday String

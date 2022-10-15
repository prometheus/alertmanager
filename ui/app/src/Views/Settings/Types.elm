module Views.Settings.Types exposing (..)

import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek)


type alias Model =
    { startOfWeek : FirstDayOfWeek
    }


type SettingsMsg
    = UpdateStartWeekAtMonday String

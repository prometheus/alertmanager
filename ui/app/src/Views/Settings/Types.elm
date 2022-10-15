module Views.Settings.Types exposing (..)

import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek)


type alias Model =
    { firstDayOfWeek : FirstDayOfWeek
    }


type SettingsMsg
    = UpdateFirstDayOfWeek String

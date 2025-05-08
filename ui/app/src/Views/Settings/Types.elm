module Views.Settings.Types exposing (..)

import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek)


type alias Model =
    { firstDayOfWeek : FirstDayOfWeek
    , bootstrapTheme : String
    }


type SettingsMsg
    = UpdateFirstDayOfWeek String
    | UpdateBootstrapTheme String

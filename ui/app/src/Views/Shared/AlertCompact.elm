module Views.Shared.AlertCompact exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (Html, div, li, span, text)
import Html.Attributes exposing (class)
import Utils.Views exposing (labelButton)


view : Alert -> Html msg
view alert =
    li [ class "mb2 w-80-l w-100-m" ] <|
        List.map
            (\( key, value ) ->
                labelButton Nothing (key ++ "=" ++ value)
            )
            alert.labels

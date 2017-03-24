module Views.Shared.AlertCompact exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (Html, span, div, text, li)
import Html.Attributes exposing (class)
import Utils.Views exposing (labelButton)


view : Int -> Alert -> Html msg
view idx alert =
    li [ class "mb2 w-80-l w-100-m" ]
        [ span [] [ text <| toString (idx + 1) ++ ". " ]
        , div [] (List.map labelButton alert.labels)
        ]

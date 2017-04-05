module Views.AlertList.FilterBar exposing (view)

import Html exposing (Html, div, span, input, text, button, i)
import Html.Attributes exposing (value, class)
import Html.Events exposing (onClick, onInput)


view : String -> (String -> msg) -> msg -> Html msg
view filterText inputChangedMsg buttonClickedMsg =
    div [ class "input-group" ]
        [ span [ class "input-group-addon" ]
            [ i [ class "fa fa-filter" ] []
            ]
        , input
            [ class "form-control", value filterText, onInput inputChangedMsg ]
            []
        , span
            [ class "input-group-btn" ]
            [ button [ class "btn btn-primary", onClick buttonClickedMsg ] [ text "Filter" ] ]
        ]

module Views.NotFound.Views exposing (view)

import Html exposing (Html, div, h1, text)
import Types exposing (Msg)


view : Html Msg
view =
    div []
        [ h1 [] [ text "not found" ]
        ]

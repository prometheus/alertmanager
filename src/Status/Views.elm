module Status.Views exposing (view)

import Html exposing (Html, h1, text)
import Types exposing (Model, Msg)

view :  model -> Html Msg
view model =
    h1 [] [ text "Status Page" ]

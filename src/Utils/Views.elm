module Utils.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)


-- Internal Imports


labelButton : ( String, String ) -> Html msg
labelButton ( key, value ) =
    listButton "light-silver hover-black ph3 pv2 " ( key, value )


listButton : String -> ( String, String ) -> Html msg
listButton classString ( key, value ) =
    a [ class <| "f6 link br1 ba mr1 mb2 dib " ++ classString ]
        [ text <| String.join "=" [ key, value ] ]

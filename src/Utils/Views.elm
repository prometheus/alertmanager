module Utils.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onCheck, onInput)


-- Internal Imports


labelButton : ( String, String ) -> Html msg
labelButton ( key, value ) =
    listButton "light-silver hover-black ph3 pv2 " ( key, value )


listButton : String -> ( String, String ) -> Html msg
listButton classString ( key, value ) =
    a [ class <| "f6 link br1 ba mr1 mb2 dib " ++ classString ]
        [ text <| String.join "=" [ key, value ] ]


checkbox : String -> Bool -> (Bool -> msg) -> Html msg
checkbox name status msg =
    label [ class "f6 dib mb2" ]
        [ input [ type_ "checkbox", checked status, onCheck msg ] []
        , text <| " " ++ name
        ]


formField : String -> String -> (String -> msg) -> Html msg
formField labelText content msg =
    div [ class "mt3" ]
        [ label [ class "f6 b db mb2" ] [ text labelText ]
        , input [ class "input-reset ba br1 b--black-20 pa2 mb2 db w-100", value content, onInput msg ] []
        ]


textField : String -> String -> (String -> msg) -> Html msg
textField labelText content msg =
    div [ class "mt3" ]
        [ label [ class "f6 b db mb2" ] [ text labelText ]
        , textarea [ class "db border-box hover-black w-100 ba b--black-20 pa2 br1 mb2", value content, onInput msg ] []
        ]

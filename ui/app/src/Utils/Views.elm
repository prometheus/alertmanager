module Utils.Views exposing (..)

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onCheck, onInput, onClick)
import Http exposing (Error(..))


onClickMsgButton : String -> msg -> Html msg
onClickMsgButton content msg =
    a [ class "f6 link br1 ba mr1 mb2 dib light-silver hover-black ph3 pv2", onClick msg ]
        [ text content ]


labelButton : ( String, String ) -> Html msg
labelButton ( key, value ) =
    listButton "light-silver hover-black ph3 pv2" ( key, value )


listButton : String -> ( String, String ) -> Html msg
listButton classString ( key, value ) =
    button classString (String.join "=" [ key, value ])


button : String -> String -> Html msg
button classes content =
    a [ class <| "f6 link br1 ba mr1 mb2 dib " ++ classes ]
        [ text content ]


iconButtonMsg : String -> String -> msg -> Html msg
iconButtonMsg classString icon msg =
    a [ class <| "f6 link br1 ba mr1 ph2 pv2 mb2 dib " ++ classString, onClick msg ]
        [ i [ class <| "fa fa-3 " ++ icon ] []
        ]


checkbox : String -> Bool -> (Bool -> msg) -> Html msg
checkbox name status msg =
    label [ class "f6 dib mb2 mr2" ]
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


buttonLink : String -> String -> String -> msg -> Html msg
buttonLink icon link color msg =
    a [ class <| "f6 link br1 ba mr1 ph3 pv2 mb2 dib " ++ color, href link, onClick msg ]
        [ i [ class <| "fa fa-3 " ++ icon ] []
        ]


formInput : String -> (String -> msg) -> Html msg
formInput inputValue msg =
    Html.input [ class "input-reset ba br1 b--black-20 pa2 mb2 mr2 dib w-40", value inputValue, onInput msg ] []


loading : Html msg
loading =
    div []
        [ i [ class "fa fa-cog fa-spin fa-3x fa-fw" ] []
        , span [ class "sr-only" ] [ text "Loading..." ]
        ]


error : Http.Error -> Html msg
error err =
    let
        msg =
            case err of
                Timeout ->
                    "timeout exceeded"

                NetworkError ->
                    "network error"

                BadStatus resp ->
                    resp.status.message ++ " " ++ resp.body

                BadPayload err resp ->
                    -- OK status, unexpected payload
                    "unexpected response from api" ++ err

                BadUrl url ->
                    "malformed url: " ++ url
    in
        div []
            [ p [] [ text <| "Error: " ++ msg ]
            ]

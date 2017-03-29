module Views.Shared.SilenceBase exposing (view)

import Html exposing (Html, div, a, p, text, b)
import Html.Attributes exposing (class, href)
import Silences.Types exposing (Silence)
import Types exposing (Msg(Noop, MsgForSilenceList))
import Views.SilenceList.Types exposing (SilenceListMsg(DestroySilence))
import Utils.Date
import Utils.Views exposing (buttonLink)
import Utils.Types exposing (Matcher)
import Utils.List


view : Silence -> Html Msg
view silence =
    let
        alertName =
            silence.matchers
                |> List.filter (\m -> m.name == "alertname")
                |> List.head
                |> Maybe.map .value
                |> Maybe.withDefault ""

        editUrl =
            String.join "/" [ "#/silences", silence.id, "edit" ]
    in
        div [ class "f6 mb3" ]
            [ a
                [ class "db link blue mb3"
                , href ("#/silences/" ++ silence.id)
                ]
                [ b [ class "db f4 mb1" ]
                    [ text alertName ]
                ]
            , div [ class "mb1" ]
                [ buttonLink "fa-pencil" editUrl "blue" Noop
                , buttonLink "fa-trash-o" "#/silences" "dark-red" (MsgForSilenceList (DestroySilence silence))
                , p [ class "dib mr2" ] [ text <| "Until " ++ Utils.Date.timeFormat silence.endsAt ]
                ]
            , div [ class "mb2 w-80-l w-100-m" ] (List.map matcherButton silence.matchers)
            ]


matcherButton : Matcher -> Html msg
matcherButton matcher =
    Utils.Views.button "light-silver hover-black ph3 pv2" <| Utils.List.mstring matcher

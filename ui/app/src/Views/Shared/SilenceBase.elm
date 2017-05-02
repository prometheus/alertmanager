module Views.Shared.SilenceBase exposing (view)

import Html exposing (Html, div, a, p, text, b)
import Html.Attributes exposing (class, href)
import Silences.Types exposing (Silence)
import Types exposing (Msg(Noop, MsgForSilenceList))
import Views.SilenceList.Types exposing (SilenceListMsg(DestroySilence, MsgForFilterBar))
import Utils.Date
import Utils.Views exposing (buttonLink, onClickMsgButton)
import Utils.Types exposing (Matcher)
import Utils.Filter
import Utils.List
import Views.FilterBar.Types as FilterBarTypes


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
                [ buttonLink "fa fa-pencil" editUrl "blue" Noop
                , buttonLink "fa fa-trash-o" "#/silences" "dark-red" (MsgForSilenceList (DestroySilence silence))
                , p [ class "dib mr2" ] [ text <| "Until " ++ Utils.Date.dateTimeFormat silence.endsAt ]
                ]
            , div [ class "mb2 w-80-l w-100-m" ] (List.map matcherButton silence.matchers)
            ]


matcherButton : Matcher -> Html Msg
matcherButton matcher =
    let
        op =
            if matcher.isRegex then
                Utils.Filter.RegexMatch
            else
                Utils.Filter.Eq
    in
        onClickMsgButton
            (Utils.List.mstring matcher)
            (FilterBarTypes.AddFilterMatcher False
                { key = matcher.name
                , op = op
                , value = matcher.value
                }
                |> MsgForFilterBar
                |> MsgForSilenceList
            )

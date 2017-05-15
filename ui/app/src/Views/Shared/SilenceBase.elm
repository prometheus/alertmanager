module Views.Shared.SilenceBase exposing (view)

import Html exposing (Html, div, a, p, text, b, i, span, small, button)
import Html.Attributes exposing (class, href, style)
import Html.Events exposing (onClick)
import Silences.Types exposing (Silence, State(Expired))
import Types exposing (Msg(Noop, MsgForSilenceList))
import Views.SilenceList.Types exposing (SilenceListMsg(DestroySilence, MsgForFilterBar))
import Utils.Date
import Utils.Views exposing (buttonLink)
import Utils.Types exposing (Matcher)
import Utils.Filter
import Utils.List
import Views.FilterBar.Types as FilterBarTypes
import Time exposing (Time)


view : Silence -> Html Msg
view silence =
    div [ class "d-inline-flex align-items-center justify-content-start w-100" ]
        [ datesView silence.startsAt silence.endsAt
        , div [ class "" ] (List.map matcherButton silence.matchers)
        , div [ class "ml-auto d-inline-flex align-self-stretch p-2", style [ ( "border-left", "1px solid #ccc" ) ] ]
            [ editButton silence.id
            , deleteButton silence
            , detailsButton silence.id
            ]
        ]


datesView : Time -> Time -> Html Msg
datesView start end =
    i [ class "d-inline-flex align-items-center", style [ ( "border-right", "1px solid #ccc" ) ] ]
        [ dateView start
        , i [ class "text-muted" ] [ text "-" ]
        , dateView end
        ]


dateView : Time -> Html Msg
dateView time =
    i
        [ class "h-100 p-2 d-flex flex-column justify-content-center, text-muted"
        , style [ ( "font-family", "monospace" ) ]
        ]
        [ span [] [ text <| Utils.Date.timeFormat time ]
        , small [] [ text <| Utils.Date.dateFormat time ]
        ]


matcherButton : Matcher -> Html Msg
matcherButton matcher =
    let
        op =
            if matcher.isRegex then
                Utils.Filter.RegexMatch
            else
                Utils.Filter.Eq

        msg =
            (FilterBarTypes.AddFilterMatcher False
                { key = matcher.name
                , op = op
                , value = matcher.value
                }
                |> MsgForFilterBar
                |> MsgForSilenceList
            )
    in
        Utils.Views.labelButton (Just msg) (Utils.List.mstring matcher)


editButton : String -> Html Msg
editButton silenceId =
    let
        editUrl =
            String.join "/" [ "#/silences", silenceId, "edit" ]
    in
        a [ class "h-100 btn btn-success rounded-0", href editUrl ]
            [ span [ class "fa fa-pencil" ] [] ]


deleteButton : Silence -> Html Msg
deleteButton silence =
    if silence.status.state == Expired then
        text ""
    else
        a
            [ class "h-100 btn btn-danger rounded-0"
            , onClick (MsgForSilenceList (DestroySilence silence))
            , href "#/silences"
            ]
            [ span [ class "fa fa-trash" ] []
            ]


detailsButton : String -> Html Msg
detailsButton silenceId =
    a [ class "h-100 btn btn-primary rounded-0", href ("#/silences/" ++ silenceId) ]
        [ span [ class "fa fa-info" ] []
        ]

module Views.SilenceList.SilenceView exposing (view)

import Html exposing (Html, div, a, p, text, b, i, span, small, button, li)
import Html.Attributes exposing (class, href, style)
import Html.Events exposing (onClick)
import Silences.Types exposing (Silence, State(Expired, Active, Pending))
import Types exposing (Msg(Noop, MsgForSilenceList, MsgForSilenceForm))
import Views.SilenceList.Types exposing (SilenceListMsg(DestroySilence, MsgForFilterBar))
import Utils.Date
import Utils.Views exposing (buttonLink)
import Utils.Types exposing (Matcher)
import Utils.Filter
import Utils.List
import Views.FilterBar.Types as FilterBarTypes
import Time exposing (Time)
import Views.SilenceForm.Types exposing (SilenceFormMsg(NewSilenceFromMatchers))


view : Silence -> Html Msg
view silence =
    li [ class "align-items-start list-group-item border-0 alert-list-item p-0 mb-4" ]
        [ div [ class "w-100 mb-2 d-flex align-items-start" ]
            [ case silence.status.state of
                Active ->
                    dateView "Ends" silence.endsAt

                Pending ->
                    dateView "Starts" silence.startsAt

                Expired ->
                    dateView "Expired" silence.endsAt
            , detailsButton silence.id
            , editButton silence
            , deleteButton silence
            ]
        , div [ class "" ] (List.map matcherButton silence.matchers)
        ]


dateView : String -> Time -> Html Msg
dateView string time =
    span
        [ class "text-muted align-self-center mr-2"
        ]
        [ text (string ++ " " ++ Utils.Date.timeFormat time ++ ", " ++ Utils.Date.dateFormat time)
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


editButton : Silence -> Html Msg
editButton silence =
    case silence.status.state of
        -- If the silence is expired, do not edit it, but instead create a new
        -- one with the old matchers
        Expired ->
            a
                [ class "btn btn-outline-info border-0"
                , href ("#/silences/new?keep=1")
                , onClick (NewSilenceFromMatchers silence.matchers |> MsgForSilenceForm)
                ]
                [ text "Recreate"
                ]

        _ ->
            let
                editUrl =
                    String.join "/" [ "#/silences", silence.id, "edit" ]
            in
                a [ class "btn btn-outline-info border-0", href editUrl ]
                    [ text "Edit"
                    ]


deleteButton : Silence -> Html Msg
deleteButton silence =
    case silence.status.state of
        Expired ->
            text ""

        Active ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceList (DestroySilence silence))
                ]
                [ text "Expire"
                ]

        Pending ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceList (DestroySilence silence))
                ]
                [ text "Delete"
                ]


detailsButton : String -> Html Msg
detailsButton silenceId =
    a [ class "btn btn-outline-info border-0", href ("#/silences/" ++ silenceId) ]
        [ text "View"
        ]

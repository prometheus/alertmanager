module Views.SilenceList.SilenceView exposing (deleteButton, editButton, view)

import Dialog
import Html exposing (Html, a, b, button, div, h3, i, li, p, small, span, text)
import Html.Attributes exposing (class, href, style)
import Html.Events exposing (onClick)
import Silences.Types exposing (Silence, State(Active, Expired, Pending))
import Time exposing (Time)
import Types exposing (Msg(MsgForSilenceForm, MsgForSilenceList, Noop))
import Utils.Date
import Utils.Filter
import Utils.List
import Utils.Types exposing (Matcher)
import Utils.Views exposing (buttonLink)
import Views.FilterBar.Types as FilterBarTypes
import Views.SilenceList.Types exposing (SilenceListMsg(ConfirmDestroySilence, DestroySilence, FetchSilences, MsgForFilterBar))
import Views.SilenceForm.Parsing exposing (newSilenceFromAlertLabels)


view : Bool -> Silence -> Html Msg
view showConfirmationDialog silence =
    li
        [ -- speedup rendering in Chrome, because list-group-item className
          -- creates a new layer in the rendering engine
          style [ ( "position", "static" ) ]
        , class "align-items-start list-group-item border-0 p-0 mb-4"
        ]
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
            , deleteButton silence False
            ]
        , div [ class "" ] (List.map matcherButton silence.matchers)
        , Dialog.view
            (if showConfirmationDialog then
                Just (confirmSilenceDeleteView silence False)
             else
                Nothing
            )
        ]


confirmSilenceDeleteView : Silence -> Bool -> Dialog.Config Msg
confirmSilenceDeleteView silence refresh =
    { closeMessage = Just (MsgForSilenceList Views.SilenceList.Types.FetchSilences)
    , containerClass = Nothing
    , header = Just (h3 [] [ text "Expire Silence" ])
    , body = Just (text "Are you sure you want to expire this silence?")
    , footer =
        Just
            (button
                [ class "btn btn-success"
                , onClick (MsgForSilenceList (Views.SilenceList.Types.DestroySilence silence refresh))
                ]
                [ text "Confirm" ]
            )
    }


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
            FilterBarTypes.AddFilterMatcher False
                { key = matcher.name
                , op = op
                , value = matcher.value
                }
                |> MsgForFilterBar
                |> MsgForSilenceList
    in
        Utils.Views.labelButton (Just msg) (Utils.List.mstring matcher)


editButton : Silence -> Html Msg
editButton silence =
    let
        matchers =
            List.map (\s -> ( s.name, s.value )) silence.matchers
    in
        case silence.status.state of
            -- If the silence is expired, do not edit it, but instead create a new
            -- one with the old matchers
            Expired ->
                a
                    [ class "btn btn-outline-info border-0"
                    , href (newSilenceFromAlertLabels matchers)
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


deleteButton : Silence -> Bool -> Html Msg
deleteButton silence refresh =
    case silence.status.state of
        Expired ->
            text ""

        Active ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceList (ConfirmDestroySilence silence refresh))
                ]
                [ text "Expire"
                ]

        Pending ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceList (ConfirmDestroySilence silence refresh))
                ]
                [ text "Delete"
                ]


detailsButton : String -> Html Msg
detailsButton silenceId =
    a [ class "btn btn-outline-info border-0", href ("#/silences/" ++ silenceId) ]
        [ text "View"
        ]

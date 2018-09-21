module Views.SilenceList.SilenceView exposing (deleteButton, editButton, view)

import Data.Matcher exposing (Matcher)
import Data.Matchers exposing (Matchers)
import Data.Silence exposing (Silence)
import Data.SilenceStatus exposing (State(..))
import Dict exposing (Dict)
import Html exposing (Html, a, b, button, div, h3, i, li, p, small, span, text)
import Html.Attributes exposing (class, href, style)
import Html.Events exposing (onClick)
import Time exposing (Posix)
import Types exposing (Msg(..))
import Utils.Date
import Utils.Filter
import Utils.List
import Utils.Views exposing (buttonLink)
import Views.FilterBar.Types as FilterBarTypes
import Views.Shared.Dialog as Dialog
import Views.SilenceForm.Parsing exposing (newSilenceFromAlertLabels)
import Views.SilenceList.Types exposing (SilenceListMsg(..))


view : Bool -> Silence -> Html Msg
view showConfirmationDialog silence =
    li
        [ -- speedup rendering in Chrome, because list-group-item className
          -- creates a new layer in the rendering engine
          style "position" "static"
        , class "align-items-start list-group-item border-0 p-0 mb-4"
        ]
        [ div [ class "w-100 mb-2 d-flex align-items-start" ]
            [ case silence.status of
                Just status ->
                    case status.state of
                        Active ->
                            dateView "Ends" silence.endsAt

                        Pending ->
                            dateView "Starts" silence.startsAt

                        Expired ->
                            dateView "Expired" silence.endsAt

                Nothing ->
                    text ""
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
    { onClose = MsgForSilenceList Views.SilenceList.Types.FetchSilences
    , title = "Expire Silence"
    , body = text "Are you sure you want to expire this silence?"
    , footer =
        button
            [ class "btn btn-primary"
            , onClick (MsgForSilenceList (Views.SilenceList.Types.DestroySilence silence refresh))
            ]
            [ text "Confirm" ]
    }


dateView : String -> Posix -> Html Msg
dateView string time =
    span
        [ class "text-muted align-self-center mr-2"
        ]
        [ text (string ++ " " ++ Utils.Date.dateTimeFormat time)
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

        editUrl =
            -- TODO: silence.id should always be set. Can this be done nicer?
            String.join "/" [ "#/silences", Maybe.withDefault "" silence.id, "edit" ]

        default =
            a [ class "btn btn-outline-info border-0", href editUrl ]
                [ text "Edit"
                ]
    in
    case silence.status of
        Just status ->
            case status.state of
                -- If the silence is expired, do not edit it, but instead create a new
                -- one with the old matchers
                Expired ->
                    a
                        [ class "btn btn-outline-info border-0"
                        , href (newSilenceFromAlertLabels <| Dict.fromList matchers)
                        ]
                        [ text "Recreate"
                        ]

                _ ->
                    default

        Nothing ->
            default


deleteButton : Silence -> Bool -> Html Msg
deleteButton silence refresh =
    -- TODO: status should always be set, how to solve this better?
    case silence.status of
        Just status ->
            case status.state of
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

        Nothing ->
            text ""


detailsButton : Maybe String -> Html Msg
detailsButton maybeSilenceId =
    -- TODO: Again, silence.id should not be Nothing here, how can this be done better?
    case maybeSilenceId of
        Just id ->
            a [ class "btn btn-outline-info border-0", href ("#/silences/" ++ id) ]
                [ text "View"
                ]

        Nothing ->
            text ""

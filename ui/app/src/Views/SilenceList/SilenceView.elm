module Views.SilenceList.SilenceView exposing (editButton, view)

import Data.GettableSilence exposing (GettableSilence)
import Data.Matcher exposing (Matcher)
import Data.SilenceStatus exposing (State(..))
import Html exposing (Html, a, button, div, li, span, text)
import Html.Attributes exposing (class, href, style)
import Html.Events exposing (onClick)
import Time exposing (Posix)
import Types exposing (Msg(..))
import Utils.Date
import Utils.Filter
import Utils.List
import Utils.Views
import Views.FilterBar.Types as FilterBarTypes
import Views.Shared.Dialog as Dialog
import Views.SilenceForm.Parsing exposing (newSilenceFromMatchersAndComment)
import Views.SilenceList.Types exposing (SilenceListMsg(..))


view : Bool -> GettableSilence -> Html Msg
view showConfirmationDialog silence =
    li
        [ -- speedup rendering in Chrome, because list-group-item className
          -- creates a new layer in the rendering engine
          style "position" "static"
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
            , deleteButton silence
            ]
        , div [ class "" ] (List.map matcherButton silence.matchers)
        , Dialog.view
            (if showConfirmationDialog then
                Just (confirmSilenceDeleteView silence False)

             else
                Nothing
            )
        ]


confirmSilenceDeleteView : GettableSilence -> Bool -> Dialog.Config Msg
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
        isEqual =
            case matcher.isEqual of
                Nothing ->
                    True

                Just value ->
                    value

        op =
            if not matcher.isRegex && isEqual then
                Utils.Filter.Eq

            else if not matcher.isRegex && not isEqual then
                Utils.Filter.NotEq

            else if matcher.isRegex && isEqual then
                Utils.Filter.RegexMatch

            else
                Utils.Filter.NotRegexMatch

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


editButton : GettableSilence -> Html Msg
editButton silence =
    let
        editUrl =
            String.join "/" [ "#/silences", silence.id, "edit" ]

        default =
            a [ class "btn btn-outline-info border-0", href editUrl ]
                [ text "Edit"
                ]
    in
    case silence.status.state of
        -- If the silence is expired, do not edit it, but instead create a new
        -- one with the old matchers
        Expired ->
            a
                [ class "btn btn-outline-info border-0"
                , href (newSilenceFromMatchersAndComment silence.matchers silence.comment)
                ]
                [ text "Recreate"
                ]

        _ ->
            default


deleteButton : GettableSilence -> Html Msg
deleteButton silence =
    case silence.status.state of
        Expired ->
            text ""

        Active ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceList (ConfirmDestroySilence silence))
                ]
                [ text "Expire"
                ]

        Pending ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceList (ConfirmDestroySilence silence))
                ]
                [ text "Delete"
                ]


detailsButton : String -> Html Msg
detailsButton id =
    a [ class "btn btn-outline-info border-0", href ("#/silences/" ++ id) ]
        [ text "View"
        ]

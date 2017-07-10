module Views.ReceiverBar.Views exposing (view)

import Html exposing (Html, li, div, text)
import Html.Attributes exposing (class, style, tabindex)
import Html.Events exposing (onBlur, onClick)
import Regex
import Views.ReceiverBar.Types exposing (Model, Msg(..))


view : Maybe String -> Model -> Html Msg
view receiver { receivers, showRecievers } =
    let
        autoCompleteClass =
            if showRecievers then
                "show"
            else
                ""

        navLinkClass =
            if showRecievers then
                "active"
            else
                ""

        -- Try to find the regex-escaped receiver in the list of unescaped receivers:
        unescapedReceiver =
            receivers
                |> List.filter (Regex.escape >> Just >> (==) receiver)
                |> List.map Just
                |> List.head
                |> Maybe.withDefault receiver
    in
        li
            [ class ("nav-item ml-auto autocomplete-menu " ++ autoCompleteClass)
            , onBlur (ToggleReceivers False)
            , tabindex 1
            , style
                [ ( "position", "relative" )
                , ( "outline", "none" )
                ]
            ]
            [ div
                [ onClick (ToggleReceivers (not showRecievers))
                , class "mt-1 mr-4"
                , style [ ( "cursor", "pointer" ) ]
                ]
                [ text ("Receiver: " ++ Maybe.withDefault "All" unescapedReceiver) ]
            , receivers
                |> List.map Just
                |> (::) Nothing
                |> List.map (receiverField unescapedReceiver)
                |> div [ class "dropdown-menu dropdown-menu-right" ]
            ]


receiverField : Maybe String -> Maybe String -> Html Msg
receiverField selected maybeReceiver =
    let
        attrs =
            if selected == maybeReceiver then
                [ class "dropdown-item active" ]
            else
                [ class "dropdown-item"
                , style [ ( "cursor", "pointer" ) ]
                , onClick (SelectReceiver maybeReceiver)
                ]
    in
        div
            attrs
            [ text (Maybe.withDefault "All" maybeReceiver) ]
